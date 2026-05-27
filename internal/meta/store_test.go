package meta

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestPinUnpin(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "meta.json"))
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	s.SetPinned("k1", true)
	if !s.Get("k1").Pinned {
		t.Error("expected pinned")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2 := New(filepath.Join(dir, "meta.json"))
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if !s2.Get("k1").Pinned {
		t.Error("expected persisted pin")
	}
}

func TestHide(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "meta.json"))
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	s.SetHidden("k", true)
	if !s.Get("k").Hidden {
		t.Error("expected hidden")
	}
}

func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.json")
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := New(path)
			_ = s.Load()
			s.SetPinned("shared", true)
			_ = s.Save()
		}(i)
	}
	wg.Wait()
	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if !s.Get("shared").Pinned {
		t.Error("expected pinned after concurrent writes")
	}
}
