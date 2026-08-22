package models

import "testing"

func TestStatusCatalogCoversValidation(t *testing.T) {
	for _, info := range StatusInfos() {
		if !IsValidStatus(string(info.Status)) {
			t.Errorf("catalog status %q rejected by IsValidStatus", info.Status)
		}
		if info.Label == "" || info.Description == "" {
			t.Errorf("status %q lacks label/description", info.Status)
		}
		switch info.Severity {
		case SeverityDanger, SeverityWarning, SeverityInfo:
		default:
			t.Errorf("status %q has unknown severity %q", info.Status, info.Severity)
		}
	}
	if IsValidStatus("bogus") {
		t.Error("IsValidStatus accepted unknown status")
	}
	if len(ValidStatusNames()) != len(StatusInfos()) {
		t.Error("ValidStatusNames out of sync with catalog")
	}
}

func TestStatusDescription(t *testing.T) {
	if _, ok := StatusDescription(StatusHacker); !ok {
		t.Error("StatusDescription missing hacker")
	}
	for _, st := range []WalletStatus{StatusPhishing, StatusScam, StatusMixer,
		StatusSanctioned, StatusExchange, StatusSuspicious, StatusFrozen} {
		if _, ok := StatusDescription(st); !ok {
			t.Errorf("StatusDescription missing %q", st)
		}
	}
	if _, ok := StatusDescription("nope"); ok {
		t.Error("StatusDescription accepted unknown status")
	}
}
