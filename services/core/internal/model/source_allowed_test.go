package model

import "testing"

func TestSourceAllowed(t *testing.T) {
	cases := []struct {
		reqType, source string
		want            bool
	}{
		{ReqTypeRoleConflictException, "identity", true},
		{ReqTypeRoleConflictException, "billing", false},
		{ReqTypeRoleConflictException, "", false},
		{ReqTypeRefund, "billing", true},
		{ReqTypeLeave, "hr", true},
	}
	for _, c := range cases {
		if got := SourceAllowed(c.reqType, c.source); got != c.want {
			t.Errorf("SourceAllowed(%q, %q) = %v, want %v", c.reqType, c.source, got, c.want)
		}
	}
	for _, typ := range KnownRequestTypes {
		if RequestTypeLabels[typ] == "" {
			t.Errorf("request type %s has no label", typ)
		}
	}
}
