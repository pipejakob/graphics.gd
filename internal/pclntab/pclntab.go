// Package pclntab resolves functions in the running binary by name, using the
// runtime's own PC/line table (the pclntab). It exists so graphics.gd can
// reach the one runtime-internal entry point it needs (runtime.asmcgocall)
// without a //go:linkname pull of a runtime symbol — linkname pulls are
// tracked by toolchain telemetry and subject to -checklinkname tightening.
//
// The header is located without any private runtime state: runtime.FuncForPC
// returns a *runtime.Func that points directly at the function's _func record
// inside the pclntab blob, and the blob begins with a magic-tagged header, so
// scanning backward from a known function's record finds it. A candidate
// header is only accepted if parsing the function table with its offsets maps
// the known function's entry back to that exact record, and a looked-up entry
// PC is only returned once runtime.FuncForPC confirms the runtime maps it to
// the same name and entry.
package pclntab

import (
	"runtime"
	"sync"
	"unsafe"
)

// pclntabMagic is abi.Go120PCLnTabMagic, the header magic emitted by Go 1.20
// through at least Go 1.26. If a future toolchain changes the table layout it
// also bumps the magic (external debuggers depend on this), so lookups fail
// cleanly rather than misparse.
const pclntabMagic = 0xfffffff1

// pcHeader mirrors runtime.pcHeader. The field that held textStart before
// Go 1.26 is retained there as padding, so the layout is identical across
// Go 1.20–1.26; text start is derived from the anchor function instead.
type pcHeader struct {
	magic          uint32
	pad1, pad2     uint8
	minLC          uint8
	ptrSize        uint8
	nfunc          int
	nfiles         uint
	_              uintptr
	funcnameOffset uintptr
	cuOffset       uintptr
	filetabOffset  uintptr
	pctabOffset    uintptr
	pclnOffset     uintptr
}

// functab mirrors runtime.functab: one entry per function, sorted by entry
// offset, at the start of the pclnOffset region. funcoff locates the _func
// record relative to that same region.
type functab struct {
	entryoff uint32
	funcoff  uint32
}

type table struct {
	hdr       *pcHeader
	base      unsafe.Pointer // address of hdr, offsets are relative to this
	textStart uintptr
}

//go:noinline
func anchorFn() {}

// find locates this module's pcHeader. The anchor is anchorFn's _func record:
// a func value's first word is the entry PC, and FuncForPC at an entry PC
// (of a //go:noinline function, so no inlining pseudo-record can intervene)
// yields the real record inside the pclntab.
func find() (table, bool) {
	fv := anchorFn
	pc := **(**uintptr)(unsafe.Pointer(&fv))
	f := runtime.FuncForPC(pc)
	if f == nil || f.Entry() != pc {
		return table{}, false
	}
	anchor := unsafe.Pointer(f)
	entryOff := *(*uint32)(anchor) // _func.entryOff
	if entryOff == ^uint32(0) || uintptr(entryOff) > pc {
		return table{}, false
	}
	textStart := pc - uintptr(entryOff)
	// The header precedes the record in the same blob, so the scan terminates
	// well before the cap; the cap only bounds a misparse on some future
	// layout so it fails instead of walking off the mapping.
	const maxScan = 1 << 30
	p := unsafe.Add(anchor, -int(uintptr(anchor)&7))
	for scanned := 0; scanned < maxScan; scanned += 8 {
		p = unsafe.Add(p, -8)
		hdr := (*pcHeader)(p)
		if hdr.magic == pclntabMagic && hdr.pad1 == 0 && hdr.pad2 == 0 &&
			hdr.minLC != 0 && hdr.ptrSize == uint8(unsafe.Sizeof(uintptr(0))) &&
			validate(hdr, p, anchor, uintptr(entryOff)) {
			return table{hdr: hdr, base: p, textStart: textStart}, true
		}
	}
	return table{}, false
}

// validate accepts a candidate header only if its function table, parsed with
// the candidate's own offsets, maps the anchor's entry offset back to the
// anchor's exact _func address — a coincidental magic match in the middle of
// table data cannot survive that round trip.
func validate(hdr *pcHeader, base, anchor unsafe.Pointer, anchorEntryOff uintptr) bool {
	if hdr.nfunc <= 0 || hdr.nfunc >= 1<<31 {
		return false
	}
	if hdr.funcnameOffset < unsafe.Sizeof(pcHeader{}) ||
		hdr.pclnOffset <= hdr.funcnameOffset ||
		hdr.pclnOffset > uintptr(anchor)-uintptr(base) {
		return false
	}
	ftab := unsafe.Add(base, hdr.pclnOffset)
	lo, hi := 0, hdr.nfunc
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		e := (*functab)(unsafe.Add(ftab, uintptr(mid)*unsafe.Sizeof(functab{})))
		if uintptr(e.entryoff) < anchorEntryOff {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo >= hdr.nfunc {
		return false
	}
	e := (*functab)(unsafe.Add(ftab, uintptr(lo)*unsafe.Sizeof(functab{})))
	return uintptr(e.entryoff) == anchorEntryOff && unsafe.Add(ftab, e.funcoff) == anchor
}

var found = sync.OnceValues(find)

// EntryPC returns the entry PC of the named function in this module. The name
// is as the runtime prints it (e.g. "runtime.asmcgocall"); note the pclntab
// does not carry ABI suffixes, so an assembly function and its linker
// generated ABI wrapper share a name and either entry may be returned.
func EntryPC(name string) (uintptr, bool) {
	t, ok := found()
	if !ok {
		return 0, false
	}
	ftab := unsafe.Add(t.base, t.hdr.pclnOffset)
	names := unsafe.Add(t.base, t.hdr.funcnameOffset)
	for i := 0; i < t.hdr.nfunc; i++ {
		e := (*functab)(unsafe.Add(ftab, uintptr(i)*unsafe.Sizeof(functab{})))
		fn := unsafe.Add(ftab, e.funcoff)
		nameOff := *(*int32)(unsafe.Add(fn, 4)) // _func.nameOff
		if nameOff < 0 || !nameEq(unsafe.Add(names, nameOff), name) {
			continue
		}
		entry := t.textStart + uintptr(e.entryoff)
		// Never hand out a PC the runtime does not confirm: the candidate
		// must round-trip through the public API to the same name and entry.
		if f := runtime.FuncForPC(entry); f != nil && f.Name() == name && f.Entry() == entry {
			return entry, true
		}
	}
	return 0, false
}

// nameEq reports whether the NUL-terminated string at p equals s.
func nameEq(p unsafe.Pointer, s string) bool {
	for i := 0; i < len(s); i++ {
		if *(*byte)(unsafe.Add(p, i)) != s[i] {
			return false
		}
	}
	return *(*byte)(unsafe.Add(p, len(s))) == 0
}
