package aprs

import "testing"

func TestParseTNC2Basic(t *testing.T) {
	p, ok := parseTNC2("DL1ABC-9>APRS,WIDE1-1,WIDE2-1,qAR,DB0ABC:!4903.50N/07201.75W-Test")
	if !ok {
		t.Fatal("expected a parse")
	}
	if p.source != "DL1ABC-9" {
		t.Fatalf("source = %q", p.source)
	}
	if p.dest != "APRS" {
		t.Fatalf("dest = %q", p.dest)
	}
	if len(p.path) != 4 || p.path[3] != "DB0ABC" {
		t.Fatalf("path = %v", p.path)
	}
	if p.info != "!4903.50N/07201.75W-Test" {
		t.Fatalf("info = %q", p.info)
	}
}

func TestParseTNC2InfoMayContainColons(t *testing.T) {
	p, ok := parseTNC2("N0CALL>APRS::TARGET   :Hello: world")
	if !ok {
		t.Fatal("expected a parse")
	}
	if p.info != ":TARGET   :Hello: world" {
		t.Fatalf("only the first colon may split the header from info, got %q", p.info)
	}
}

func TestParseTNC2Rejects(t *testing.T) {
	cases := []string{"", "no angle bracket here", "N0CALL>nocolon", ">EMPTYSOURCE:!x", "N0CALL>:emptydest:x"}
	for _, line := range cases {
		if _, ok := parseTNC2(line); ok {
			t.Fatalf("expected rejection for %q", line)
		}
	}
}

func TestLastPathHopStripsRepeatedMarker(t *testing.T) {
	p, ok := parseTNC2("N0CALL>APRS,WIDE1-1,qAR,DB0ABC*:!test")
	if !ok {
		t.Fatal("expected a parse")
	}
	if hop := p.lastPathHop(); hop != "DB0ABC" {
		t.Fatalf("lastPathHop = %q, want DB0ABC", hop)
	}
}

func TestLastPathHopEmptyWhenNoPath(t *testing.T) {
	p, ok := parseTNC2("N0CALL>APRS:!test")
	if !ok {
		t.Fatal("expected a parse")
	}
	if hop := p.lastPathHop(); hop != "" {
		t.Fatalf("lastPathHop = %q, want empty", hop)
	}
}
