import { Schema } from "effect";

/** Timestamp strings decode to UTC; retain the published plain-string JSON schema.
 * Calendar validity belongs to the codec and Go time decoding, not a newly
 * introduced JSON Schema format restriction in an existing contract major.
 */
export const Timestamp = Schema.DateTimeUtcFromString;
