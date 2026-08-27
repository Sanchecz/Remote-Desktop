import assert from "node:assert/strict";
import test from "node:test";

import {
	browserCodeToVirtualKey,
	chunkRemoteText,
	planRemoteKeyboardInput,
	planRemoteTextReconciliation,
} from "../src/remoteKeyboard.ts";

const event = (overrides: Partial<{ code: string; key: string; ctrlKey: boolean; altKey: boolean; metaKey: boolean }> = {}) => ({
	code: "KeyA",
	key: "a",
	ctrlKey: false,
	altKey: false,
	metaKey: false,
	...overrides,
});

test("maps navigation, punctuation, numpad and modifier physical keys", () => {
	assert.equal(browserCodeToVirtualKey("ShiftLeft"), 16);
	assert.equal(browserCodeToVirtualKey("Slash"), 191);
	assert.equal(browserCodeToVirtualKey("Quote"), 222);
	assert.equal(browserCodeToVirtualKey("Numpad7"), 103);
	assert.equal(browserCodeToVirtualKey("NumpadAdd"), 107);
	assert.equal(browserCodeToVirtualKey("CapsLock"), 20);
});

test("sends Shift punctuation as its resolved Unicode symbol", () => {
	const textKeys = new Set<string>();
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "ShiftLeft", key: "Shift" }), "down", textKeys), {
		handled: true,
		input: { type: "key", action: "down", keyCode: 16 },
	});
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "Slash", key: "?" }), "down", textKeys), {
		handled: true,
		input: { type: "text", text: "?" },
	});
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "Slash", key: "?" }), "up", textKeys), { handled: true });
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "ShiftLeft", key: "Shift" }), "up", textKeys), {
		handled: true,
		input: { type: "key", action: "up", keyCode: 16 },
	});
});

test("preserves Ctrl shortcuts as physical key events", () => {
	const textKeys = new Set<string>();
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "KeyC", key: "c", ctrlKey: true }), "down", textKeys), {
		handled: true,
		input: { type: "key", action: "down", keyCode: 67 },
	});
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "KeyC", key: "c", ctrlKey: false }), "up", textKeys), {
		handled: true,
		input: { type: "key", action: "up", keyCode: 67 },
	});
});

test("sends local case and Cyrillic independently from the remote layout", () => {
	const textKeys = new Set<string>();
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "KeyF", key: "А" }), "down", textKeys).input, { type: "text", text: "А" });
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "Digit1", key: "!" }), "down", textKeys).input, { type: "text", text: "!" });
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "Equal", key: "+" }), "down", textKeys).input, { type: "text", text: "+" });
});

test("does not synthesize dead or unidentified composition keys", () => {
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "", key: "Dead" }), "down", new Set()), { handled: false });
	assert.deepEqual(planRemoteKeyboardInput(event({ code: "", key: "Unidentified" }), "down", new Set()), { handled: false });
});

test("chunks pasted Unicode text by code points without splitting surrogate pairs", () => {
	const source = "а".repeat(127) + "👋" + "бв";
	const chunks = chunkRemoteText(source);
	assert.deepEqual(chunks.map((chunk) => Array.from(chunk).length), [128, 2]);
	assert.equal(chunks.join(""), source);
});

test("streams appended mobile IME text without waiting for Enter", () => {
	assert.deepEqual(planRemoteTextReconciliation("Прив", "Привет"), { backspaces: 0, text: "ет" });
});

test("reconciles Android IME corrections using remote Backspace taps", () => {
	assert.deepEqual(planRemoteTextReconciliation("превет", "привет"), { backspaces: 4, text: "ивет" });
	assert.deepEqual(planRemoteTextReconciliation("hello", "hell"), { backspaces: 1, text: "" });
});

test("does not split emoji while reconciling mobile input", () => {
	assert.deepEqual(planRemoteTextReconciliation("готов 👋", "готов ✅"), { backspaces: 1, text: "✅" });
});
