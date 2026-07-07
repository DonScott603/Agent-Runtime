# RFC-0001: Canonical Message Schema

Status: ACCEPTED — 2026-07-06. Resolved: D1=(a), D2=(a), D3=(a), with
declared downgrades available on profile opt-in
Change policy: core is additive-only after v1. Changes to this document require RFC review. This is the runtime's ABI; treat every field as something third-party code and years-old log events will depend on.

## 1. Purpose and position

The canonical message schema is the single representation of conversational content shared by every component of the runtime: context plugins produce it, model adapters translate it, the event log persists it, replay folds it, UI clients render it, evaluators score it. It is deliberately provider-neutral. Provider dialects (Anthropic, OpenAI, local runtimes) exist only inside adapters, which translate to and from this form at the broker boundary.

Because the log persists messages indefinitely, this schema is an ABI across time: a message written under v1 must parse and fold correctly under every future version. All guarantees downstream — replay, audit, retroactive views — depend on this property.

## 2. Core model (frozen after v1)

A `Message` is:

    Message {
      id:             MessageId          // unique within run
      role:           Role               // see D1
      content:        [ContentBlock]     // ordered, never reordered
      provenance:     Provenance         // producing principal + step ref
      schema_version: SemVer
    }

A `ContentBlock` is:

    ContentBlock {
      id:    BlockId                     // unique within message
      type:  BlockType                   // namespaced string, see §3
      body:  <type-specific payload>     // canonical serialization per D2
    }

Large or binary bodies (media, documents, provider-raw payloads) are not embedded; they are content-addressed blob references (`core.blob`). See RFC-0002 §6 and decision D10 for what this implies for message text bodies.

### D1 — Role model and tool-result placement [RESOLVED: (a)]

Options:
  (a) Roles = {system, user, assistant}; tool results are `core.tool_result`
      blocks carried in a user-role message (Anthropic-style).
  (b) Roles = {system, user, assistant, tool}; tool results are messages
      with role=tool (OpenAI-style).

DECISION (accepted): (a). Tool results as blocks keeps the pairing invariant
(§4-I2) local to the block layer, translates losslessly to both major
dialects, and avoids a role whose semantics differ per provider. Adapters
targeting role=tool dialects synthesize the role during translation.

## 3. Block types

Core block types (every consumer MUST understand):

    core.text          { text: string }
    core.tool_use      { tool_use_id, capability, operation, input }
    core.tool_result   { tool_use_id, output | error, taint: Provenance }
    core.blob          { blob_hash, media_type, byte_len, purpose }

Everything else is an extension type, namespaced:

    ext.<vendor>.<name>     e.g. ext.anthropic.thinking, ext.openai.audio
    x.<user>.<name>         reserved for local/experimental use

New provider features land as new extension types — never as mutations of
core types. Promotion from ext.* to core.* is an RFC-level event.

### Passthrough law (normative)

A component that does not understand a block type MUST preserve the block
byte-for-byte in canonical serialization, in its original position. A
component may remove or transform blocks only for types it explicitly
declares transform capability over. Passthrough is CI-enforced: the
conformance suite injects unknown block types through every registered
plugin and adapter and asserts identity.

Rationale: the silent-strip failure mode (a summarizer discarding thinking
or cache-hint blocks it doesn't recognize) corrupts provider state and
replay fidelity with no visible error.

## 4. Invariants (part of the ABI)

  I1. Block order within a message is produced-order and is never
      reordered by any component.
  I2. Every core.tool_use is answered by exactly one core.tool_result
      bearing the same tool_use_id, in a later message of the same run.
      An unanswered tool_use is only legal in a run that is suspended,
      failed, or cancelled.
  I3. Block ids are unique within their message; message ids within
      their run.
  I4. A message, once appended to the log, is immutable. Corrections are
      new events (see RFC-0002), never edits.
  I5. schema_version is present on every message and every block whose
      type carries its own version.

Validators for I1–I5 ship with the kernel and run on every append.

## 5. Canonical serialization

### D2 — Serialization format [RESOLVED: (a)]

The event hash chain (RFC-0002 §4) hashes canonical bytes, so
serialization must be deterministic: one message, one byte sequence.

Options:
  (a) JSON with JCS-style canonicalization (RFC 8785): sorted keys,
      no insignificant whitespace, integers only (no floats in core;
      decimals as strings), UTF-8 NFC.
  (b) CBOR deterministic encoding (RFC 8949 §4.2).
  (c) Protobuf (rejected: no canonical serialization guarantee across
      library versions).

DECISION (accepted): (a). Human-inspectable logs are worth the space cost for
a single-owner system whose audit story is "read the log"; JCS rules are
implementable in ~a page of Go. Binary content never enters serialization
(blobs), which removes JSON's worst cases.

## 6. Adapter contract

An adapter declares, in its manifest:
  - supported block types (core is mandatory; ext.* enumerated)
  - supported invariant extensions (parallel tool_use, streaming)
  - a fidelity class per supported type: lossless | lossy(reason)

Round-trip requirement: for every supported type,
canonical → provider → canonical MUST be byte-identical (fuzz-tested in CI
with generated messages).

### D3 — Behavior on unsupported blocks [RESOLVED: (a) + declared (c)]

When a conversation contains blocks the target adapter does not support:

Options:
  (a) Fail closed: the model call errors; routing/policy must pick a
      capable adapter.
  (b) Drop with warning event.
  (c) Downgrade map: adapter provides a declared, deterministic
      transformation (e.g. thinking → omitted; audio → transcript blob).

DECISION (accepted): (a) by default, with (c) available only when the adapter
declares an explicit downgrade for that type and the profile opts in.
Never (b): silent loss is the failure mode this schema exists to prevent.
The declared downgrade is itself recorded (plugin-hash + transform id) so
replay can attribute the loss.

## 7. Raw escape hatch

Adapters MAY attach the provider-native request/response as a core.blob
(purpose = "provider-raw") referenced from the message provenance. The
canonical form remains the event of record; the raw form exists for
disputes and forensics. Raw blobs participate in erasure like any blob.

## 8. Versioning and upcasting

  - Core: additive-only. New optional fields; never removed or
    re-typed fields; never re-purposed values.
  - Extensions: versioned independently by their namespace owner.
  - Every reader tolerates unknown fields (must-ignore) and unknown
    block types (passthrough law).
  - Upcasters (vN → vN+1) are maintained forever and are pure
    functions registered with the kernel; they run at fold time,
    never by rewriting the log.

## 9. Conformance suite (ships with the kernel)

  1. Round-trip fuzzing per adapter per supported type.
  2. Passthrough injection through every plugin.
  3. Invariant validators I1–I5 over generated and recorded corpora.
  4. Canonical-bytes stability: serialize twice across library
     versions, compare hashes.
  5. Upcaster totality: every historical corpus message upcasts to
     current version without error.
