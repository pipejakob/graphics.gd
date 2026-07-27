package gdfunc

import (
	"fmt"
	"io"
	"strings"

	"graphics.gd/internal/gdjson"
	"graphics.gd/internal/tool/generate/gdtype"
)

// TrivialMethods maps class name → set of method names that are trivial
// (zero function calls in C++ body). When set, Generate emits jumponly.Call
// for these methods instead of noescape.Call.
var TrivialMethods map[string]map[string]bool

// ThreadSafeSingletons are the engine singletons whose methods the engine
// guards internally (command queues or locks), per
// https://docs.godotengine.org/en/stable/tutorials/performance/thread_safe_apis.html
// Their generated bindings call the engine directly from any thread instead
// of routing through the cross-thread dispatch ring. The map value names an
// atomic.Bool in package gd that gates the direct dispatch at runtime, for
// singletons that are only thread-safe under certain project settings; empty
// means unconditionally thread-safe.
var ThreadSafeSingletons = map[string]string{
	"RenderingServer":    "", // Godot 4: off-render-thread calls go through its locked command queue
	"NavigationServer2D": "", // documented as "thread-safe and thread-friendly"
	"NavigationServer3D": "",
	"ResourceLoader":     "", // loading resources from threads is supported
	"ResourceSaver":      "",
	"WorkerThreadPool":   "", // built for use from threads
	"PhysicsServer2D":    "PhysicsServer2DThreadSafe", // only when run_on_separate_thread is enabled
	"PhysicsServer3D":    "PhysicsServer3DThreadSafe",
}

// allocatingResults are engine return types whose value is *constructed* into
// the ptrcall return buffer and may heap-allocate (CoW containers, Strings,
// Variant). jumponly runs the C++ method on the goroutine stack without the cgo
// stack switch, so returning one of these can corrupt the musl allocator even
// when the method body looks call-free to the trivial-methods analyzer — the
// allocation happens in the binder's return-value construction, not the body
// (e.g. GLTFState.get_nodes is a "trivial" accessor but get_nodes_bind builds a
// TypedArray<GLTFNode>). Object/value-type returns are a plain store and stay on
// the fast path; these never do.
var allocatingResults = map[string]bool{
	"gdextension.String":             true,
	"gdextension.StringName":         true,
	"gdextension.NodePath":           true,
	"gdextension.Array":              true,
	"gdextension.Dictionary":         true,
	"gdextension.Variant":            true,
	"gdextension.PackedByteArray":    true,
	"gdextension.PackedInt32Array":   true,
	"gdextension.PackedInt64Array":   true,
	"gdextension.PackedFloat32Array": true,
	"gdextension.PackedFloat64Array": true,
	"gdextension.PackedStringArray":  true,
	"gdextension.PackedVector2Array": true,
	"gdextension.PackedVector3Array": true,
	"gdextension.PackedColorArray":   true,
	"gdextension.PackedVector4Array": true,
}

type Type int

const (
	TypeDefault Type = iota
	TypeBuiltin
	TypeUtility
	TypeVarargs
)

// anchored reports whether a Go value of the given engine type carries a
// garbage-collected anchor that frees an engine-side resource when it is
// collected, and so has to outlive an engine call that took a raw pointer out
// of it. Plain value types (numbers, enums, the math structs, RIDs) hold
// nothing the collector can reclaim and are left alone, so that the common
// buffered call keeps its arguments in registers.
func anchored(classDB map[string]gdjson.Class, gdType string) bool {
	if class, ok := classDB[gdType]; ok {
		return !class.IsEnum
	}
	if strings.HasPrefix(gdType, "typedarray::") {
		return true
	}
	switch gdType {
	case "String", "StringName", "NodePath", "Array", "Dictionary", "Variant",
		"Callable", "Signal", "Object", "PackedByteArray", "PackedInt32Array",
		"PackedInt64Array", "PackedFloat32Array", "PackedFloat64Array",
		"PackedStringArray", "PackedVector2Array", "PackedVector3Array",
		"PackedVector4Array", "PackedColorArray":
		return true
	default:
		return false
	}
}

// keepalives emits the [runtime.KeepAlive] calls that have to follow an engine
// call. Everything the call handed to the engine as a raw pointer — the
// receiver and any reference-typed argument — is dead, as far as the collector
// is concerned, from the moment that pointer was extracted. Off the main thread
// the call is only *recorded* in the cross-thread ring at this point, and the
// frees those wrappers queue when they are collected are ordered behind the
// recorded calls, so a collection inside that window queues the free first and
// the engine frees the object before running the call that uses it.
//
// The receiver is kept alive through its one-word anchor rather than as a whole
// wrapper: keeping the wrapper live spills four words across every call (~1.5ns
// on a ~15ns buffered call), the anchor stays in a register.
func keepalives(w io.Writer, classDB map[string]gdjson.Class, method gdjson.Method, singleton bool) {
	// Singletons are process-lifetime and are never collected, so their
	// receiver needs no anchoring (matching the unchecked ObjectChecked).
	if !method.IsStatic && !singleton {
		fmt.Fprint(w, "\truntime.KeepAlive(self[0].Anchor())\n")
	}
	for _, arg := range method.Arguments {
		if !anchored(classDB, arg.Type) {
			continue
		}
		// Class-typed arguments arrive as a one element array of the wrapper
		// ([1]gdclass.Node, [1]gdreference.Object), both of which carry the
		// same one-word anchor as the receiver.
		if _, isClass := classDB[arg.Type]; isClass {
			fmt.Fprintf(w, "\truntime.KeepAlive(%s[0].Anchor())\n", fixReserved(arg.Name))
			continue
		}
		fmt.Fprintf(w, "\truntime.KeepAlive(%s)\n", fixReserved(arg.Name))
	}
}

func fixReserved(name string) string {
	switch name {
	case "bool":
		return "b"
	case "type":
		return "atype"
	case "range":
		return "arange"
	case "default":
		return "def"
	case "class":
		return "class_"
	case "func":
		return "fn"
	case "frame":
		return "frame_"
	case "interface":
		return "intf"
	case "internal":
		return "internal_"
	case "map":
		return "mapping"
	case "var":
		return "v"
	case "object":
		return "obj"
	case "string":
		return "s"
	case "RID":
		return "rid"
	case "variant":
		return "v"
	default:
		return name
	}
}

func Generate(w io.Writer, classDB map[string]gdjson.Class, pkg string, class gdjson.Class, method gdjson.Method, ctype Type, setter_getter, singleton bool) {
	if ctype == TypeDefault && method.IsVararg {
		ctype = TypeVarargs
	}
	switch class.Name {
	case "Float", "Int", "Vector2", "Vector2i", "Rect2", "Rect2i", "Vector3", "Vector3i",
		"Transform2D", "Vector4", "Vector4i", "Plane", "Quaternion", "AABB", "Basis", "Transform3D",
		"RID", "Projection", "Color":
		return
	}
	result := gdtype.EngineTypeAsAddressable(class.Name, method.Name, "", method.ReturnValue.Meta, method.ReturnValue.Type)
	if method.ReturnType != "" {
		result = gdtype.EngineTypeAsAddressable(class.Name, method.Name, "", "", method.ReturnType)
	}
	ptrKind, isPtr := gdtype.Name(result).IsPointer()

	prefix := ""
	if pkg != "internal" {
		prefix = "gd."
	}
	if method.IsVirtual {
		fmt.Fprintf(w, "func (class) %s(impl func(ptr gdclass.Receiver", method.Name)
		for _, arg := range method.Arguments {
			fmt.Fprint(w, ", ")
			fmt.Fprintf(w, "%v %v", fixReserved(arg.Name), gdtype.EngineTypeAsAddressable(class.Name, method.Name, arg.Name, arg.Meta, arg.Type))
		}
		fmt.Fprintf(w, ") %v) (cb "+prefix+"ExtensionClassCallVirtualFunc) {\n", result)
		fmt.Fprint(w, "\treturn func(class any, p_args, p_back gdextension.Pointer) {\n")
		var hasPointerBarrier bool
		for i, arg := range method.Arguments {
			var argType = gdtype.EngineTypeAsAddressable(class.Name, method.Name, arg.Name, arg.Meta, arg.Type)
			pointerKind, argIsPtr := gdtype.Name(argType).IsPointer()
			if !argIsPtr {
				pointerKind = argType
			}
			// Callback arguments are borrowed engine references, released
			// by the deferred EndPointer below. Pin them so a concurrent
			// main-thread Cycle can't free them mid-callback (the callback
			// may run off-thread); EndPointer still frees the pin.
			fmt.Fprintf(w, "\t\tvar %v = %v\n", fixReserved(arg.Name), gdtype.Name(argType).LoadFromRawPointerValuePinned(
				fmt.Sprintf("gd.UnsafeGet[%v](p_args,%d)", pointerKind, i),
			))
			if argIsPtr {
				if strings.HasPrefix(string(argType), "Engine.Pointer[") && !hasPointerBarrier {
					fmt.Fprintf(w, "\t\tdefer %s\n", gdtype.Name(argType).EndPointer(fixReserved(arg.Name)))
					hasPointerBarrier = true
				} else if !strings.HasPrefix(string(argType), "Engine.Pointer[") {
					fmt.Fprintf(w, "\t\tdefer %s\n", gdtype.Name(argType).EndPointer(fixReserved(arg.Name)))
				}
			}
		}
		fmt.Fprintf(w, "\t\tself := gdclass.ReceiverOf(class)\n")
		if result != "" {
			fmt.Fprintf(w, "\t\tret := ")
		}
		fmt.Fprintf(w, "impl(self")
		for _, arg := range method.Arguments {
			fmt.Fprint(w, ", ")
			fmt.Fprintf(w, "%v", fixReserved(arg.Name))
		}
		fmt.Fprintf(w, ")\n")
		if result != "" {
			ret := gdtype.Name(result).ToUnderlying("ret")
			if strings.HasPrefix(result, "Engine.Pointer[") {
				// Engine.Pointer returns are unwrapped back to gdextension.Pointer.
				fmt.Fprintf(w, "\t\t"+prefix+"UnsafeSet(p_back, %s)\n", gdtype.Name(result).CallframeValue(ret))
			} else if isPtr {
				fmt.Fprintf(w, "ptr, ok := %s\n", gdtype.Name(result).EndPointer(ret))
				fmt.Fprintf(w, "\n\t\tif !ok {\n")
				fmt.Fprintf(w, "\t\t\treturn\n")
				fmt.Fprintf(w, "\t\t}\n")
				// Godot pre-initialises Array/Dictionary return slots; assign
				// over them via UnsafeReplace* so the pre-allocated value is
				// destroyed rather than leaked (godotengine/godot#119440).
				// Mirrors the v2 generator's simpleVirtualCall.
				switch {
				case result == "Dictionary.Any":
					fmt.Fprintf(w, "\t\t%sUnsafeReplaceDictionary(p_back, ptr)\n", prefix)
				case result == "Array.Any" || strings.HasPrefix(result, "Array.Contains["):
					fmt.Fprintf(w, "\t\t%sUnsafeReplaceArray(p_back, ptr)\n", prefix)
				default:
					fmt.Fprintf(w, "\t\t%sUnsafeSet(p_back, ptr)\n", prefix)
				}
			} else {
				fmt.Fprintf(w, "\t\t"+prefix+"UnsafeSet(p_back, %s)\n", ret)
			}
		}
		fmt.Fprintf(w, "\t}\n")
		fmt.Fprintf(w, "}\n")
		return
	}
	if ctype == TypeBuiltin && strings.HasPrefix(class.Name, "Packed") {
		fmt.Fprintf(w, "\nfunc (self *class) %v(", gdjson.ConvertName(method.Name))
	} else {
		fmt.Fprintf(w, "\nfunc (self class) %v(", gdjson.ConvertName(method.Name))
	}

	if method.Name == "select" {
		method.Name = "select_"
	}
	if method.Name == "map" {
		method.Name = "map_"
	}

	for i, arg := range method.Arguments {
		if i > 0 {
			fmt.Fprint(w, ", ")
		}
		fmt.Fprintf(w, "%v %v", fixReserved(arg.Name), gdtype.EngineTypeAsAddressable(class.Name, method.Name, arg.Name, arg.Meta, arg.Type))
	}
	if method.IsVararg {
		if len(method.Arguments) > 0 {
			fmt.Fprint(w, ", ")
		}
		fmt.Fprintf(w, "args ...%sVariant", prefix)
	}
	fmt.Fprintf(w, ") %v { //gd:%s.%s\n", result, class.Name, method.Name)
	if singleton {
		fmt.Fprintf(w, "once.Do(singleton)\n\t")
	}
	var static = ""
	if method.IsStatic {
		static = "Static"
	}
	var self = " gd.ObjectChecked(self.AsObject()),"
	if singleton {
		self = " gdreference.GetObject(self.AsObject()[0])," // singletons don't need to be checked.
	}
	if method.IsStatic {
		self = ""
	}
	var callResult = result
	if isPtr {
		callResult = ptrKind
	}
	if ctype == TypeVarargs {
		// The variants are held as wrappers and only unwrapped into the packed
		// argument list at the point of the call, so that they can be kept
		// alive across it: unwrapping them straight into the list would leave
		// the call carrying raw pointers to variants the collector is free to
		// destroy (see keepalives).
		fmt.Fprintf(w, "var fixed = [...]%sVariant{", prefix)
		for i, arg := range method.Arguments {
			if i > 0 {
				fmt.Fprint(w, ", ")
			}
			fmt.Fprintf(w, "%sNewVariant(%s)", prefix, fixReserved(arg.Name))
		}
		fmt.Fprint(w, "}\n")
		fmt.Fprintf(w, "var dynamic []%sVariant\n", prefix)
		fmt.Fprintln(w, "for _, arg := range args {")
		fmt.Fprintf(w, "\tdynamic = append(dynamic, %sNewVariant(arg))\n", prefix)
		fmt.Fprintln(w, "}")
		fmt.Fprintln(w, "var packed = make([]gdextension.Variant, 0, len(fixed)+len(dynamic))")
		fmt.Fprintln(w, "for _, arg := range fixed {")
		fmt.Fprintln(w, "\tpacked = append(packed, gdextension.Variant(pointers.Get(arg)))")
		fmt.Fprintln(w, "}")
		fmt.Fprintln(w, "for _, arg := range dynamic {")
		fmt.Fprintln(w, "\tpacked = append(packed, gdextension.Variant(pointers.Get(arg)))")
		fmt.Fprintln(w, "}")
		fmt.Fprintf(w, "\tret, err := noescape.MethodForClass(methods.%v).Call%s(%s packed...)\n", method.Name, static, self)
		keepalives(w, classDB, method, singleton)
		fmt.Fprintln(w, "\truntime.KeepAlive(fixed)")
		fmt.Fprintln(w, "\truntime.KeepAlive(dynamic)")
		fmt.Fprintf(w, "\tif err != nil {\n")
		fmt.Fprintf(w, "\t\tpanic(err)\n")
		fmt.Fprintf(w, "\t}\n")
		if result != "" {
			fmt.Fprintf(w, "\treturn gd.VariantAs[%s](pointers.New[gd.Variant]([3]uint64(ret)))\n", result)
		} else {
			fmt.Fprintf(w, "\t_ = ret\n")
		}
		fmt.Fprintf(w, "}\n")
		return
	}
	if result != "" {
		fmt.Fprintf(w, "\tvar r_ret = ")
	} else {
		callResult = "struct{}"
	}
	callPkg := "noescape"
	callName := "Call" + static
	callFlag := ""
	if flag, threadSafe := ThreadSafeSingletons[class.Name]; threadSafe && !method.IsStatic {
		// thread-safe singleton: cross into the engine directly from any
		// thread instead of routing through the cross-thread dispatch ring.
		if flag == "" {
			callName = "CallThreadSafe"
		} else {
			callName = "CallThreadSafeIf"
			callFlag = " &" + prefix + flag + ","
		}
	} else if TrivialMethods != nil && TrivialMethods[class.Name][method.Name] && !allocatingResults[callResult] {
		callPkg = "jumponly"
	}
	fmt.Fprintf(w, "%s.%s[%s](%s%s methods.%v, %v, &struct{", callPkg, callName, callResult, callFlag, self, method.Name, shapeOf(class, method))
	for i, arg := range method.Arguments {
		if i > 0 {
			fmt.Fprint(w, "; ")
		}
		argType := gdtype.EngineTypeAsAddressable(class.Name, method.Name, arg.Name, arg.Meta, arg.Type)
		fmt.Fprintf(w, "%s %s", fixReserved(arg.Name), gdtype.Name(argType).CallframeType())
	}
	fmt.Fprint(w, "}{")
	for i, arg := range method.Arguments {
		if i > 0 {
			fmt.Fprint(w, ", ")
		}
		_, ok := classDB[arg.Type]
		if ok {
			switch semantics := gdjson.ClassMethodOwnership[class.Name][method.Name][arg.Name]; semantics {
			case gdjson.OwnershipTransferred, gdjson.LifetimeBoundToClass:
				fmt.Fprintf(w, "\tgdextension.Object(gd.PointerWithOwnershipTransferredToGodot(gdclass.Get%v(%v[0])[0]))", arg.Type, fixReserved(arg.Name))
			case gdjson.IsTemporaryReference, gdjson.MustAssertInstanceID, gdjson.ReversesTheOwnership:
				fmt.Fprintf(w, "\tgdextension.Object(gdreference.GetObject(gdclass.Get%v(%v[0])[0]))", arg.Type, fixReserved(arg.Name))
			case gdjson.RefCountedManagement:
				fmt.Fprintf(w, "\tgdextension.Object(gdreference.GetObject(gdclass.Get%v(%v[0])[0]))", arg.Type, fixReserved(arg.Name))
			default:
				panic("unknown ownership: " + fmt.Sprint(semantics))
			}
			continue
		}
		argType := gdtype.EngineTypeAsAddressable(class.Name, method.Name, arg.Name, arg.Meta, arg.Type)
		fmt.Fprint(w, gdtype.Name(argType).CallframeValue(fixReserved(arg.Name)))
	}
	fmt.Fprint(w, "})\n")
	keepalives(w, classDB, method, singleton)
	if gdjson.Flushables[class.Name+"."+method.Name] {
		fmt.Fprint(w, "\tgd.Flush()\n")
	}
	if isPtr {
		_, ok := classDB[strings.TrimPrefix(result, "[1]gdclass.")]
		if ok || result == "[1]gdreference.Object" {
			if result == "[1]gdreference.Object" {
				switch semantics := gdjson.ClassMethodOwnership[class.Name][method.Name]["return value"]; semantics {
				case gdjson.RefCountedManagement, gdjson.OwnershipTransferred:
					fmt.Fprintf(w, "\tvar ret = [1]gdreference.Object{%sPointerWithOwnershipTransferredToGo(r_ret)}\n", prefix)
				case gdjson.LifetimeBoundToClass:
					fmt.Fprintf(w, "\tvar ret = [1]gdreference.Object{%sPointerLifetimeBoundTo(self.AsObject(), r_ret)}\n", prefix)
				case gdjson.MustAssertInstanceID:
					fmt.Fprintf(w, "\tvar ret = [1]gdreference.Object{gdreference.LetObject(r_ret)}\n")
				default:
					panic("unknown ownership: " + fmt.Sprint(semantics))
				}
			} else {
				switch semantics := gdjson.ClassMethodOwnership[class.Name][method.Name]["return value"]; semantics {
				case gdjson.RefCountedManagement, gdjson.OwnershipTransferred:
					fmt.Fprintf(w, "\tvar ret = [1]gdclass.%s{gdclass.New%[1]s("+prefix+"PointerWithOwnershipTransferredToGo(r_ret))}\n", method.ReturnValue.Type)
				case gdjson.LifetimeBoundToClass:
					fmt.Fprintf(w, "\tvar ret = [1]gdclass.%s{gdclass.New%[1]s("+prefix+"PointerLifetimeBoundTo(self.AsObject(), r_ret))}\n", method.ReturnValue.Type)
				case gdjson.MustAssertInstanceID:
					fmt.Fprintf(w, "\tvar ret = [1]gdclass.%s{gdclass.New%[1]s(gdreference.LetObject(r_ret))}\n", method.ReturnValue.Type)
				case gdjson.IsTemporaryReference:
					fmt.Fprintf(w, "\tvar ret = [1]gdclass.%s{gdclass.New%[1]s("+prefix+"PointerBorrowedTemporarily(r_ret))}\n", method.ReturnValue.Type)
				default:
					panic("unknown ownership: " + fmt.Sprint(semantics))
				}
			}
		} else {
			fmt.Fprintf(w, "\tvar ret = %s\n", gdtype.Name(result).LoadFromRawPointerValue("r_ret"))
		}
	} else if result != "" {
		fmt.Fprintf(w, "\tvar ret = %s\n", gdtype.Name(result).LoadFromRawPointerValue("r_ret"))
	}

	if method.Name == "queue_free" {
		fmt.Fprintf(w, "\tgd.PointerQueueFree(self.AsObject()[0])\n")
	}

	if result != "" {
		if strings.HasPrefix(result, "ArrayOf") || strings.HasPrefix(result, "gd.ArrayOf") {
			result = strings.ReplaceAll(result, "gd.ArrayOf", "gd.TypedArray")
			result = strings.ReplaceAll(result, "ArrayOf", "TypedArray")
			fmt.Fprintf(w, "\treturn %s(ret)\n", result)
		} else {
			fmt.Fprintf(w, "\treturn ret\n")
		}
	}
	/*if result != "" {
		fmt.Fprintf(w, "\tvar ret %v\n", result)
		fmt.Fprintf(w, "\treturn ret\n")
	}*/
	fmt.Fprintf(w, "}")
}

func shapeOf(class gdjson.Class, method gdjson.Method) string {
	var result = method.ReturnValue.Type
	if result == "" {
		result = method.ReturnType
	}
	var shape string
	if result != "" {
		shape += sizeOf(class.Name, method.ReturnValue.Meta, result)
	} else {
		shape = "0"
	}
	for i, arg := range method.Arguments {
		shape += "|(" + sizeOf(class.Name, arg.Meta, arg.Type) + "<<" + fmt.Sprint(4*(i+1)) + ")"
	}
	return shape
}

func sizeOf(name, meta, gdType string) string {
	if strings.HasPrefix(gdType, "typedarray::") {
		return "gdextension.SizeArray"
	}
	switch gdType {
	case "int", "Int":
		return "gdextension.SizeInt"
	case "float", "Float":
		return "gdextension.SizeFloat"
	case "bool", "Bool":
		return "gdextension.SizeBool"
	case "StringName", "Vector2", "Vector2i", "Rect2", "Rect2i", "Vector3", "Vector3i", "Transform2D",
		"Vector4", "Vector4i", "Plane", "Quaternion", "AABB", "Basis", "Transform3D", "Projection", "Color", "RID",
		"NodePath", "Signal", "Array", "Dictionary", "String", "Callable", "Variant", "Object":
		return "gdextension.Size" + gdType
	case "PackedInt32Array", "PackedInt64Array", "PackedFloat32Array", "PackedFloat64Array", "PackedVector2Array", "PackedVector3Array", "PackedVector4Array", "PackedColorArray", "PackedStringArray", "PackedByteArray":
		return "gdextension.SizePackedArray"
	// strange C++ cases
	case "const uint8_t **", "const void*", "const uint8_t*", "const uint8_t *", "float*", "int32_t*", "void*", "uint8_t*":
		return "gdextension.SizePointer"
	default:
		if strings.HasPrefix(gdType, "enum::") || strings.HasPrefix(gdType, "bitfield::") {
			return "gdextension.SizeInt"
		}
		return "gdextension.SizeObject"
	}
}
