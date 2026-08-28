import assert from "node:assert/strict";
import test from "node:test";

import { buildCodexOperatorInstruction, buildWindowsMCPInstaller } from "../src/codexSetup.ts";

const token = `rmt_mcp_${"A".repeat(43)}`;

test("builds a one-file Windows installer with verification and no manual command assembly", () => {
	const script = buildWindowsMCPInstaller("https://supportgenesis.ru/settings", token);
	assert.match(script, /RemoteIt-MCP\.exe/);
	assert.match(script, /SHA256SUMS\.txt/);
	assert.match(script, /Get-FileHash -Algorithm SHA256/);
	assert.match(script, /mcp add remoteit/);
	assert.match(script, /OpenAI\\Codex\\bin/);
	assert.match(script, /"%CODEX_CMD%" mcp add remoteit/);
	assert.match(script, /set "REMOTEIT_URL=https:\/\/supportgenesis\.ru"/);
	assert.match(script, new RegExp(token));
	assert.match(script, /del \/q/);
	assert.match(script, /del \/q "%~f0" & exit \/b 0/);
});

test("rejects unsafe server addresses and malformed integration tokens", () => {
	assert.throws(() => buildWindowsMCPInstaller("http://example.org", token), /HTTPS/);
	assert.throws(() => buildWindowsMCPInstaller("https://user:pass@example.org", token), /Invalid/);
	assert.throws(() => buildWindowsMCPInstaller("https://supportgenesis.ru", "not-a-token"), /token/);
});

test("builds a reusable instruction for another Codex without embedding a token", () => {
	const instruction = buildCodexOperatorInstruction("https://supportgenesis.ru");
	assert.match(instruction, /Remote ID/);
	assert.match(instruction, /script\.execute/);
	assert.match(instruction, /владельца или администратора/);
	assert.doesNotMatch(instruction, /rmt_mcp_/);
});
