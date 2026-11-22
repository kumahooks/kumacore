package httpx_test

import (
	"testing"

	"kumacore/core/httpx"
)

func TestParsePage_Empty_ReturnsOne(t *testing.T) {
	if page := httpx.ParsePaginationPage(""); page != 1 {
		t.Errorf("got %d, want 1", page)
	}
}

func TestParsePage_One_ReturnsOne(t *testing.T) {
	if page := httpx.ParsePaginationPage("1"); page != 1 {
		t.Errorf("got %d, want 1", page)
	}
}

func TestParsePage_Two_ReturnsTwo(t *testing.T) {
	if page := httpx.ParsePaginationPage("2"); page != 2 {
		t.Errorf("got %d, want 2", page)
	}
}

func TestParsePage_Zero_ReturnsOne(t *testing.T) {
	if page := httpx.ParsePaginationPage("0"); page != 1 {
		t.Errorf("got %d, want 1", page)
	}
}

func TestParsePage_Negative_ReturnsOne(t *testing.T) {
	if page := httpx.ParsePaginationPage("-1"); page != 1 {
		t.Errorf("got %d, want 1", page)
	}
}

func TestParsePage_NonNumeric_ReturnsOne(t *testing.T) {
	cases := []string{"abc", "1.5", "2abc", " ", "0x10", "1e3"}
	for _, raw := range cases {
		if page := httpx.ParsePaginationPage(raw); page != 1 {
			t.Errorf("ParsePage(%q): got %d, want 1", raw, page)
		}
	}
}

func TestParsePage_HugeOverflow_ReturnsOne(t *testing.T) {
	if page := httpx.ParsePaginationPage("99999999999999999999999999"); page != 1 {
		t.Errorf("huge overflow: got %d, want 1", page)
	}
}

func TestParsePage_PlusSign_AtoiAccepts(t *testing.T) {
	if page := httpx.ParsePaginationPage("+2"); page != 2 {
		t.Errorf("got %d, want 2 (Atoi accepts leading +)", page)
	}
}

func TestParsePage_SQLInjectionPayload_ReturnsOne(t *testing.T) {
	cases := []string{
		"' OR '1'='1",
		"1; DROP TABLE users",
		"1 UNION SELECT 1",
		"--",
		"';--",
	}
	for _, raw := range cases {
		if page := httpx.ParsePaginationPage(raw); page != 1 {
			t.Errorf("injection %q: got %d, want 1 (non-numeric clamps)", raw, page)
		}
	}
}
