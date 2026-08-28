package main

import "fmt"

// Windows defines SoftwareSASGeneration as a two-bit policy:
// 0 = disabled, 1 = services, 2 = ease-of-access applications, 3 = both.
// RemoteIt only needs the service bit and must preserve the other bit while the
// installed service is available for secure-attention requests.
func desiredSoftwareSASGeneration(current uint64, exists bool) (uint32, bool, error) {
	if !exists {
		return 1, true, nil
	}
	if current > 3 {
		return 0, false, fmt.Errorf("unsupported SoftwareSASGeneration value %d", current)
	}
	target := uint32(current) | 1
	return target, uint64(target) != current, nil
}

func windowsSASEventName(kind string, sessionID uint32) string {
	return fmt.Sprintf(`Global\RemoteIt-SAS-%s-%d`, kind, sessionID)
}

// SendSAS receives TRUE when the calling thread runs in the current
// interactive user's security context. The RemoteIt broker impersonates the
// target Windows session before the call, so AsUser must be TRUE. Passing
// FALSE targets the unimpersonated service context and can silently do nothing
// because SendSAS has no return value. Keeping the conversion outside the
// Windows-only file makes this contract regression-testable on every host.
func windowsSASAsUserArgument(callingAsCurrentUser bool) uintptr {
	if callingAsCurrentUser {
		return 1
	}
	return 0
}

// The SendSAS contract distinguishes a real SCM service from a normal process.
// A LocalSystem service addressing the physical console must use AsUser=FALSE.
// Non-console RDS/VDI sessions still need the explicitly impersonated helper
// path, otherwise Windows can deliver the sequence to the wrong session.
func windowsSASShouldImpersonate(targetSessionID, consoleSessionID uint32) bool {
	return targetSessionID != consoleSessionID
}
