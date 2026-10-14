import { Schema } from "effect";

/** Export strict Draft 2020-12 files with locally resolvable shared definitions. */
export function compileJsonSchema(schema: Schema.Constraint): Readonly<Record<string, unknown>> {
  // Publish the declared wire representation, not a new canonical JSON codec
  // for decoded Effect values. Published optionality and refinements belong
  // to the wire side; timestamp calendar validation remains in its codec.
  const document = Schema.toJsonSchemaDocument(Schema.toEncoded(schema), {
    onExcessProperty: "error",
  });
  return {
    $schema: "https://json-schema.org/draft/2020-12/schema",
    ...document.schema,
    ...(Object.keys(document.definitions).length === 0 ? {} : { $defs: document.definitions }),
  };
}
