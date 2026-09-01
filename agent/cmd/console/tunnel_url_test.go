package main

import "testing"

func TestParseRemoteItTunnelURL(t *testing.T) {
	request, err := parseRemoteItTunnelURL("remoteit://connect?id=11111111-2222-3333-4444-555555555555&token=abcdefghijklmnopqrstuvwxyz012345&protocol=rdp&username=DOMAIN%5Cuser")
	if err != nil {
		t.Fatal(err)
	}
	if request.Protocol != "rdp" || request.Username != `DOMAIN\user` {
		t.Fatalf("unexpected request: %#v", request)
	}
	for _, raw := range []string{
		"https://connect/?id=11111111-2222-3333-4444-555555555555&token=abcdefghijklmnopqrstuvwxyz012345&protocol=rdp",
		"remoteit://connect?id=bad&token=abcdefghijklmnopqrstuvwxyz012345&protocol=rdp",
		"remoteit://connect?id=11111111-2222-3333-4444-555555555555&token=short&protocol=ssh",
		"remoteit://connect?id=11111111-2222-3333-4444-555555555555&token=abcdefghijklmnopqrstuvwxyz012345&protocol=vnc",
		"remoteit://connect?id=11111111-2222-3333-4444-555555555555&token=abcdefghijklmnopqrstuvwxyz012345&protocol=ssh&username=-oProxyCommand%3Dbad",
	} {
		if _, err := parseRemoteItTunnelURL(raw); err == nil {
			t.Fatalf("unsafe URL accepted: %s", raw)
		}
	}
}
