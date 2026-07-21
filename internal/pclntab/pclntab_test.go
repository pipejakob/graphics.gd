package pclntab

import (
	"runtime"
	"testing"
)

func TestFind(t *testing.T) {
	if _, ok := found(); !ok {
		t.Fatal("pcHeader not found")
	}
}

func TestEntryPC(t *testing.T) {
	for _, name := range []string{
		"runtime.asmcgocall",
		"runtime.cgocall",
		"runtime.mallocgc",
		"graphics.gd/internal/pclntab.TestEntryPC",
	} {
		pc, ok := EntryPC(name)
		if !ok {
			t.Fatalf("EntryPC(%q) not found", name)
		}
		f := runtime.FuncForPC(pc)
		if f == nil {
			t.Fatalf("EntryPC(%q) = %#x: FuncForPC is nil", name, pc)
		}
		if f.Name() != name || f.Entry() != pc {
			t.Fatalf("EntryPC(%q) = %#x: runtime says name=%q entry=%#x", name, pc, f.Name(), f.Entry())
		}
	}
}

func TestEntryPCUnknown(t *testing.T) {
	if pc, ok := EntryPC("graphics.gd/no.such.Function"); ok {
		t.Fatalf("EntryPC of nonexistent function = %#x, want not found", pc)
	}
}
