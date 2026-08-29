export type RemoteKeyboardInput =
	| { type: "key"; action: "down" | "up"; keyCode: number }
	| { type: "text"; text: string };

export type RemoteKeyboardEvent = {
	code: string;
	key: string;
	ctrlKey: boolean;
	altKey: boolean;
	metaKey: boolean;
};

export type RemoteKeyboardPlan = {
	handled: boolean;
	input?: RemoteKeyboardInput;
};

export type RemoteTextReconciliation = {
	backspaces: number;
	text: string;
};

export type RemoteBoundaryDeletion = {
	handled: boolean;
	keyCode?: 8 | 46;
};

// Browser `code` names describe physical keys, whereas Windows SendInput uses
// virtual-key values. Keep this mapping for shortcuts, navigation and other
// stateful keys. Printable input follows the Unicode path below so Shift
// punctuation and non-Latin layouts do not depend on the remote keyboard
// layout being identical to the administrator's computer.
export function browserCodeToVirtualKey(code: string): number {
	if (/^Key[A-Z]$/.test(code)) return code.charCodeAt(3);
	if (/^Digit[0-9]$/.test(code)) return code.charCodeAt(5);
	if (/^Numpad[0-9]$/.test(code)) return 96 + Number(code.slice(6));
	if (/^F([1-9]|1[0-2])$/.test(code)) return 111 + Number(code.slice(1));
	return ({
		Backspace: 8,
		Tab: 9,
		Enter: 13,
		NumpadEnter: 13,
		ShiftLeft: 16,
		ShiftRight: 16,
		ControlLeft: 17,
		ControlRight: 17,
		AltLeft: 18,
		AltRight: 18,
		Pause: 19,
		CapsLock: 20,
		Escape: 27,
		Space: 32,
		PageUp: 33,
		PageDown: 34,
		End: 35,
		Home: 36,
		ArrowLeft: 37,
		ArrowUp: 38,
		ArrowRight: 39,
		ArrowDown: 40,
		PrintScreen: 44,
		Insert: 45,
		Delete: 46,
		MetaLeft: 91,
		MetaRight: 92,
		ContextMenu: 93,
		NumpadMultiply: 106,
		NumpadAdd: 107,
		NumpadSubtract: 109,
		NumpadDecimal: 110,
		NumpadDivide: 111,
		NumLock: 144,
		ScrollLock: 145,
		Semicolon: 186,
		Equal: 187,
		Comma: 188,
		Minus: 189,
		Period: 190,
		Slash: 191,
		Backquote: 192,
		BracketLeft: 219,
		Backslash: 220,
		BracketRight: 221,
		Quote: 222,
	} as Record<string, number>)[code] || 0;
}

export function isPrintableRemoteKey(key: string): boolean {
	return key !== "Dead" && key !== "Unidentified" && Array.from(key).length === 1;
}

// `textKeys` records keys whose keydown was sent as Unicode. Their physical
// keyup must be consumed locally: sending a virtual-key release for a key that
// was never pressed remotely can disturb modifier state in some applications.
export function planRemoteKeyboardInput(
	event: RemoteKeyboardEvent,
	action: "down" | "up",
	textKeys: Set<string>,
): RemoteKeyboardPlan {
	const physicalKey = event.code || event.key;
	if (action === "up" && textKeys.delete(physicalKey)) return { handled: true };

	const printable = isPrintableRemoteKey(event.key);
	const commandModified = event.ctrlKey || event.altKey || event.metaKey;
	if (action === "down" && printable && !commandModified) {
		textKeys.add(physicalKey);
		return { handled: true, input: { type: "text", text: event.key } };
	}

	const keyCode = browserCodeToVirtualKey(event.code);
	if (!keyCode) return { handled: false };
	return { handled: true, input: { type: "key", action, keyCode } };
}

export function chunkRemoteText(text: string, maxRunes = 128): string[] {
	if (maxRunes < 1) throw new Error("maxRunes must be positive");
	const runes = Array.from(text);
	const chunks: string[] = [];
	for (let index = 0; index < runes.length; index += maxRunes) chunks.push(runes.slice(index, index + maxRunes).join(""));
	return chunks;
}

// Android IMEs frequently replace the composing suffix instead of emitting a
// reliable key event for every character. Reconcile the visible input value
// against the last value already delivered to the remote machine. Work in
// Unicode code points so emoji and supplementary characters are never split.
export function planRemoteTextReconciliation(previous: string, next: string): RemoteTextReconciliation {
	const previousCodePoints = Array.from(previous);
	const nextCodePoints = Array.from(next);
	let sharedPrefix = 0;
	while (
		sharedPrefix < previousCodePoints.length
		&& sharedPrefix < nextCodePoints.length
		&& previousCodePoints[sharedPrefix] === nextCodePoints[sharedPrefix]
	) sharedPrefix += 1;
	return {
		backspaces: previousCodePoints.length - sharedPrefix,
		text: nextCodePoints.slice(sharedPrefix).join(""),
	};
}

// A controlled mobile input can only reconcile characters that it currently
// mirrors. Once its local value is empty, Android still emits `beforeinput`
// for Backspace but the value itself cannot change, so `input` never fires.
// Intercept only that boundary case and send a real remote editing key. When
// local text exists on the side being deleted, normal IME reconciliation must
// remain authoritative to avoid sending the same deletion twice.
export function planRemoteBoundaryDeletion(
	inputType: string,
	value: string,
	selectionStart: number | null,
	selectionEnd: number | null,
): RemoteBoundaryDeletion {
	const start = selectionStart ?? value.length;
	const end = selectionEnd ?? start;
	if (start !== end) return { handled: false };
	if (inputType.endsWith("Backward") && start === 0) return { handled: true, keyCode: 8 };
	if (inputType.endsWith("Forward") && end === value.length) return { handled: true, keyCode: 46 };
	return { handled: false };
}
