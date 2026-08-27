package main

import "fmt"

// Windows defines SoftwareSASGeneration as a two-bit policy:
// 0 = disabled, 1 = services, 2 = ease-of-access applications, 3 = both.
// RemoteIt only needs the service bit and must preserve the other bit while the
// secure-attention request is being dispatched.
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
