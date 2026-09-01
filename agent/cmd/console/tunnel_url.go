package main

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var remoteItTunnelIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type remoteItTunnelRequest struct {
	ID       string
	Token    string
	Protocol string
	Username string
}

func parseRemoteItTunnelURL(raw string) (remoteItTunnelRequest, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "remoteit") || !strings.EqualFold(parsed.Host, "connect") {
		return remoteItTunnelRequest{}, errors.New("ссылка подключения RemoteIt некорректна")
	}
	request := remoteItTunnelRequest{
		ID:       strings.ToLower(strings.TrimSpace(parsed.Query().Get("id"))),
		Token:    strings.TrimSpace(parsed.Query().Get("token")),
		Protocol: strings.ToLower(strings.TrimSpace(parsed.Query().Get("protocol"))),
		Username: strings.TrimSpace(parsed.Query().Get("username")),
	}
	if !remoteItTunnelIDPattern.MatchString(request.ID) || len(request.Token) < 24 || len(request.Token) > 512 || (request.Protocol != "rdp" && request.Protocol != "ssh") || !validRemoteItTunnelUsername(request.Username) || strings.IndexFunc(request.Token, unicode.IsControl) >= 0 {
		return remoteItTunnelRequest{}, errors.New("параметры подключения RemoteIt не прошли проверку")
	}
	return request, nil
}

func validRemoteItTunnelUsername(value string) bool {
	return len(value) <= 255 && !strings.HasPrefix(value, "-") && strings.IndexFunc(value, unicode.IsControl) < 0
}
