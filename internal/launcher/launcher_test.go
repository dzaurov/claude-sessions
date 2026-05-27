package launcher

import (
	"reflect"
	"testing"
)

func TestBuildArgs_minimal(t *testing.T) {
	got := BuildArgs(Options{UUID: "u1"})
	want := []string{"claude", "--resume", "u1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildArgs_withDefaultArgs(t *testing.T) {
	got := BuildArgs(Options{
		UUID:        "u1",
		DefaultArgs: []string{"--dangerously-skip-permissions"},
	})
	want := []string{"claude", "--resume", "u1", "--dangerously-skip-permissions"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildArgs_fork(t *testing.T) {
	got := BuildArgs(Options{UUID: "u1", ForkSession: true})
	want := []string{"claude", "--resume", "u1", "--fork-session"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildArgs_forkAndDefaultArgs(t *testing.T) {
	got := BuildArgs(Options{
		UUID:        "u1",
		ForkSession: true,
		DefaultArgs: []string{"--model", "opus"},
	})
	want := []string{"claude", "--resume", "u1", "--fork-session", "--model", "opus"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildArgs_multipleDefaultArgs(t *testing.T) {
	got := BuildArgs(Options{
		UUID:        "u1",
		DefaultArgs: []string{"--dangerously-skip-permissions", "--model", "opus"},
	})
	want := []string{"claude", "--resume", "u1", "--dangerously-skip-permissions", "--model", "opus"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
