import assert from "node:assert/strict";
import test from "node:test";
import { Schema } from "effect";
import { compileJsonSchema } from "../scripts/json-schema.js";
import { LocalKey } from "./repository-descriptor-v1.js";
import { Timestamp } from "./timestamp.js";

test("portable compiler preserves patterns and closes nested objects", () => {
  const document = compileJsonSchema(Schema.Struct({ nested: Schema.Struct({ key: LocalKey }) }));
  const serialized = JSON.stringify(document);
  assert.match(serialized, /"pattern":"\^\[a-z\]/);
  assert.equal((serialized.match(/"additionalProperties":false/g) ?? []).length, 2);
  const decode = Schema.decodeUnknownResult(
    Schema.Struct({ nested: Schema.Struct({ key: LocalKey }) }),
    { onExcessProperty: "error" },
  );
  assert.equal(decode({ nested: { key: "valid", extra: true } })._tag, "Failure");
  assert.equal(decode({ nested: { key: "Invalid" } })._tag, "Failure");
});

test("wire codecs preserve timestamp strings and optional versus nullable fields", () => {
  const schema = Schema.Struct({
    timestamp: Timestamp,
    absent: Schema.optionalKey(Schema.String),
    nullable: Schema.NullOr(Schema.String),
  });
  const decode = Schema.decodeUnknownResult(schema, { onExcessProperty: "error" });
  assert.equal(decode({ timestamp: "2026-10-13T09:30:00Z", nullable: null })._tag, "Success");
  assert.equal(decode({ timestamp: "invalid", nullable: null })._tag, "Failure");
  assert.equal(decode({ timestamp: "2026-10-13T09:30:00Z" })._tag, "Failure");
  const document = compileJsonSchema(schema);
  // The legacy DateTimeUtc definition was a plain JSON string. Adding a
  // format here would narrow an existing major during a dependency upgrade.
  assert.match(JSON.stringify(document), /"timestamp":\{"type":"string"\}/);
  assert.doesNotMatch(JSON.stringify(document), /"format":"date-time"/);
  assert.deepEqual(document.required, ["timestamp", "nullable"]);
});

test("portable compiler retains inclusive numeric and array bounds", () => {
  const schema = Schema.Array(
    Schema.Int.check(Schema.isBetween({ minimum: 0, maximum: 10_000 })),
  ).check(Schema.isMinLength(1), Schema.isMaxLength(3));
  const document = compileJsonSchema(schema);
  assert.equal(document.minItems, 1);
  assert.equal(document.maxItems, 3);
  const serialized = JSON.stringify(document);
  assert.match(serialized, /"minimum":0/);
  assert.match(serialized, /"maximum":10000/);
  const decode = Schema.decodeUnknownResult(schema);
  for (const input of [[], [-1], [10_001], [0.5], [0, 1, 2, 3]]) {
    assert.equal(decode(input)._tag, "Failure");
  }
  assert.equal(decode([0, 10_000])._tag, "Success");
});
