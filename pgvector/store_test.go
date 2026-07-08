package pgvector

import (
	"testing"
)

func TestFloat32SliceToVectorString(t *testing.T) {
	v := []float32{0.1, 0.2, 0.3}
	s := float32SliceToVectorString(v)
	if s != "[0.1,0.2,0.3]" {
		t.Errorf("expected '[0.1,0.2,0.3]', got %q", s)
	}
}

func TestFloat32SliceToVectorString_Empty(t *testing.T) {
	v := []float32{}
	s := float32SliceToVectorString(v)
	if s != "[]" {
		t.Errorf("expected '[]', got %q", s)
	}
}

func TestFloat32SliceToFloat32(t *testing.T) {
	s := "[0.1,0.2,0.3]"
	v, err := float32SliceToFloat32(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(v))
	}
}

func TestFloat32SliceToFloat32_Empty(t *testing.T) {
	s := "[]"
	v, err := float32SliceToFloat32(s)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v) != 0 {
		t.Fatalf("expected 0 elements, got %d", len(v))
	}
}

func TestFloat32SliceToFloat32_Invalid(t *testing.T) {
	s := "[0.1,invalid,0.3]"
	_, err := float32SliceToFloat32(s)
	if err == nil {
		t.Fatal("expected error for invalid float")
	}
}
