package kernel

import (
	"strings"
	"testing"
)

func TestParseFullDecisionResponse_RejectsEmptyReply(t *testing.T) {
	for _, body := range []string{"", "   ", "\n\t"} {
		full, err := parseFullDecisionResponse(body, DecisionSizing{})
		if err == nil {
			t.Fatalf("empty reply %q must be a failed call, got decisions: %+v", body, full.Decisions)
		}
		if !strings.Contains(err.Error(), "empty response") {
			t.Fatalf("error must identify the empty reply, got: %v", err)
		}
		if full == nil || len(full.Decisions) != 0 {
			t.Fatalf("empty reply must not produce fallback decisions, got: %+v", full)
		}
	}
}

func TestParseFullDecisionResponse_NonEmptyNonJSONStillFallsBack(t *testing.T) {
	full, err := parseFullDecisionResponse("the model only wrote prose", DecisionSizing{})
	if err != nil {
		t.Fatalf("prose without JSON must still use the safe fallback: %v", err)
	}
	if full == nil || len(full.Decisions) != 1 {
		t.Fatalf("expected one safe wait decision, got: %+v", full)
	}
	if full.Decisions[0].Action != "wait" || full.Decisions[0].Symbol != "ALL" {
		t.Fatalf("unexpected fallback decision: %+v", full.Decisions[0])
	}
}

func TestParseFullDecisionResponse_RejectsTruncatedDecisionPayload(t *testing.T) {
	full, err := parseFullDecisionResponse("<reasoning>cut off</reasoning>\n<decision>\n```json\n[{\"symbol\":\"BTCUSDT\"\n```\n</decision>", DecisionSizing{})
	if err == nil {
		t.Fatalf("truncated structured output must fail, got decisions: %+v", full.Decisions)
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("error must identify the truncation, got: %v", err)
	}
}

func TestParseFullDecisionResponse_AcceptsEmptyDecisionArray(t *testing.T) {
	full, err := parseFullDecisionResponse("<decision>[]</decision>", DecisionSizing{})
	if err != nil {
		t.Fatalf("an empty decision array means no action, not a failure: %v", err)
	}
	if full == nil || len(full.Decisions) != 0 {
		t.Fatalf("expected zero decisions, got: %+v", full)
	}
}
