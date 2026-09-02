//go:build windows

package main

import "testing"

func TestParseWindowsNetBIOSHostName(t *testing.T) {
	output := []byte("NetBIOS Remote Machine Name Table\r\n\r\n    Name               Type         Status\r\n    ---------------------------------------------\r\n    GEN-HV       <00>  UNIQUE      Registered\r\n    WORKGROUP    <00>  GROUP       Registered\r\n")
	if got := parseWindowsNetBIOSHostName(output); got != "GEN-HV" {
		t.Fatalf("unexpected NetBIOS computer name: %q", got)
	}
}

func TestParseWindowsNetBIOSHostNameRejectsMalformedName(t *testing.T) {
	if got := parseWindowsNetBIOSHostName([]byte("unsafe/name <00> UNIQUE Registered")); got != "" {
		t.Fatalf("unsafe NetBIOS name accepted: %q", got)
	}
}
