# Annotated Trace: one run, end to end

Every event below is hashed with the real rules (canon.json, chain.json);
the chain verifies. Blob hashes are illustrative placeholders (real ones
cover ciphertext). This trace is the seed scenario for the corpus
generator and the tie-breaker for small ambiguities the RFCs leave open.

### seq 101 - `run.created`

The owner (or a schedule acting with owner authority) creates the run. The principal on every subsequent agent-authored event will be agent:research - kernel-assigned, never self-declared.

```json
{
  "seq": 101,
  "run_id": "run_0042",
  "event_id": "42920793ba6c4ba0cafc496485c6163349806f009032e3b1d408718d43e3938d",
  "prev_hash": "0000000000000000000000000000000000000000000000000000000000000000",
  "ts": 1751790000,
  "mono": 11,
  "principal": "owner",
  "type": "run.created",
  "type_version": 1,
  "payload": {
    "agent": "research",
    "profile": "work"
  },
  "blobs": [],
  "sig": null
}
```

### seq 102 - `run.started`

State machine: created -> running. Transitions are events; current state is a fold.

```json
{
  "seq": 102,
  "run_id": "run_0042",
  "event_id": "dbee04017b6fe940cf129e18034ff5874859f0b4bdf1375417db7922db51580a",
  "prev_hash": "42920793ba6c4ba0cafc496485c6163349806f009032e3b1d408718d43e3938d",
  "ts": 1751790000,
  "mono": 12,
  "principal": "service:kernel",
  "type": "run.started",
  "type_version": 1,
  "payload": {},
  "blobs": [],
  "sig": null
}
```

### seq 103 - `msg.appended`

D10: the text body lives in an encrypted blob; the event carries structure plus the blob hash. (Blob hashes in this trace are illustrative - real ones cover ciphertext.)

```json
{
  "seq": 103,
  "run_id": "run_0042",
  "event_id": "43ff2eebb60514db6b2568a61e776dd674f871ed4f5c1d6e0578ba43a72785c8",
  "prev_hash": "dbee04017b6fe940cf129e18034ff5874859f0b4bdf1375417db7922db51580a",
  "ts": 1751790001,
  "mono": 13,
  "principal": "owner",
  "type": "msg.appended",
  "type_version": 1,
  "payload": {
    "message_id": "m1",
    "role": "user",
    "blocks": [
      {
        "id": "b1",
        "type": "core.text",
        "body_blob": "d5916246c2c55d13cfe05f4686b369aecc9eda7f9d2eb4a050dbde3696fa5bc7"
      }
    ]
  },
  "blobs": [
    "d5916246c2c55d13cfe05f4686b369aecc9eda7f9d2eb4a050dbde3696fa5bc7"
  ],
  "sig": null
}
```

### seq 104 - `plugin.invoked`

Pure plugin invocation recorded with its code hash (RFC-0004 P5). Verify-mode replay re-executes this and diffs against the recorded output; hash mismatch vs re-execution mismatch distinguishes version drift from nondeterminism.

```json
{
  "seq": 104,
  "run_id": "run_0042",
  "event_id": "dea8ab5417f845b949a9f88f767f447afabfdc35f8895f975c71f55fe36ad29a",
  "prev_hash": "43ff2eebb60514db6b2568a61e776dd674f871ed4f5c1d6e0578ba43a72785c8",
  "ts": 1751790001,
  "mono": 14,
  "principal": "service:kernel",
  "type": "plugin.invoked",
  "type_version": 1,
  "payload": {
    "plugin_id": "ctx-default",
    "code_hash": "c0de000000000000000000000000000000000000000000000000000000000000",
    "interface": "ContextProvider",
    "output_blob": "9306fb9a9da044155fbb742871c4ca585cdd3013c64889f8915a6a942e2070b5"
  },
  "blobs": [
    "9306fb9a9da044155fbb742871c4ca585cdd3013c64889f8915a6a942e2070b5"
  ],
  "sig": null
}
```

### seq 105 - `effect.proposed`

The frozen payload. Everything the broker will execute is fixed here, before any decision.

```json
{
  "seq": 105,
  "run_id": "run_0042",
  "event_id": "a1cf5e3774f81ac8999b4213498c9af60695463dcfeabebabc763919e852b973",
  "prev_hash": "dea8ab5417f845b949a9f88f767f447afabfdc35f8895f975c71f55fe36ad29a",
  "ts": 1751790002,
  "mono": 15,
  "principal": "agent:research",
  "type": "effect.proposed",
  "type_version": 1,
  "payload": {
    "capability": "model",
    "operation": "call",
    "input": {
      "adapter": "anthropic",
      "request_blob": "cbc52baea37291929b1933f0ddadbe9ad71311c49d61c8f3e34e0af2854db93c"
    }
  },
  "blobs": [
    "cbc52baea37291929b1933f0ddadbe9ad71311c49d61c8f3e34e0af2854db93c"
  ],
  "sig": null
}
```

### seq 106 - `gate.decision`

Resolution recorded with the winning rule AND the candidate set - every decision is explainable in isolation (RFC-0003 s9.4).

```json
{
  "seq": 106,
  "run_id": "run_0042",
  "event_id": "7515858fff8ab7fc64689458547c3a0c04829d2b0433e2cb17b7332155f7de1a",
  "prev_hash": "a1cf5e3774f81ac8999b4213498c9af60695463dcfeabebabc763919e852b973",
  "ts": 1751790002,
  "mono": 16,
  "principal": "service:kernel",
  "type": "gate.decision",
  "type_version": 1,
  "payload": {
    "effect_seq": 105,
    "action": "allow",
    "rule_id": "POL-007",
    "derived_scopes": [
      {
        "capability": "model",
        "operation": "call",
        "qualifiers": {
          "adapter": "anthropic"
        }
      }
    ],
    "candidates": [
      "POL-007"
    ]
  },
  "blobs": [],
  "sig": null
}
```

### seq 107 - `effect.executed`

The isolation descriptor names what was ACTUALLY applied (D12) - the audit trail never overstates confinement.

```json
{
  "seq": 107,
  "run_id": "run_0042",
  "event_id": "2fbadc8b04cfe57160647b0d957f52b0ddd10b0744fc802c206d72e753649bb9",
  "prev_hash": "7515858fff8ab7fc64689458547c3a0c04829d2b0433e2cb17b7332155f7de1a",
  "ts": 1751790002,
  "mono": 17,
  "principal": "service:broker",
  "type": "effect.executed",
  "type_version": 1,
  "payload": {
    "effect_seq": 105,
    "idempotency_key": "idem-105",
    "isolation": {
      "platform": "linux",
      "mechanisms": [
        "landlock",
        "seccomp",
        "netns"
      ]
    }
  },
  "blobs": [],
  "sig": null
}
```

### seq 108 - `effect.result`

Model output enters the log as tainted data. From here on, everything downstream of it is inside the influence boundary.

```json
{
  "seq": 108,
  "run_id": "run_0042",
  "event_id": "74739102a7635b1614fd0a41a9a0eb96e2203879e0ae89830040cb3e20206cb6",
  "prev_hash": "2fbadc8b04cfe57160647b0d957f52b0ddd10b0744fc802c206d72e753649bb9",
  "ts": 1751790005,
  "mono": 18,
  "principal": "service:broker",
  "type": "effect.result",
  "type_version": 1,
  "payload": {
    "effect_seq": 105,
    "output_blob": "6cce097bec52959cf0cc4ebcad78a7a82f85de63f047bd4bbd9b5f41110f1329",
    "taint": {
      "source": "model"
    }
  },
  "blobs": [
    "6cce097bec52959cf0cc4ebcad78a7a82f85de63f047bd4bbd9b5f41110f1329"
  ],
  "sig": null
}
```

### seq 109 - `msg.appended`

D1: tool_use is a block, not a role. Invariant I2 now obliges exactly one core.tool_result for t1 before this run may complete.

```json
{
  "seq": 109,
  "run_id": "run_0042",
  "event_id": "17a619f0b434122e5b2ccf31f9e6ee6357b80ad24326532f59bdd33a73aeba6b",
  "prev_hash": "74739102a7635b1614fd0a41a9a0eb96e2203879e0ae89830040cb3e20206cb6",
  "ts": 1751790005,
  "mono": 19,
  "principal": "agent:research",
  "type": "msg.appended",
  "type_version": 1,
  "payload": {
    "message_id": "m2",
    "role": "assistant",
    "blocks": [
      {
        "id": "b1",
        "type": "core.text",
        "body_blob": "550bc469a640b8e958261e5c7ca12e58788205009f33a9e1ffae634f12b9acdf"
      },
      {
        "id": "b2",
        "type": "core.tool_use",
        "tool_use_id": "t1",
        "capability": "email",
        "operation": "send",
        "input_blob": "bafd56a34ffcbbd8014024d7d4be152b3daa9a7f7e3b1c2f7f55643f722dad76"
      }
    ]
  },
  "blobs": [
    "550bc469a640b8e958261e5c7ca12e58788205009f33a9e1ffae634f12b9acdf",
    "bafd56a34ffcbbd8014024d7d4be152b3daa9a7f7e3b1c2f7f55643f722dad76"
  ],
  "sig": null
}
```

### seq 110 - `effect.proposed`

Second effect: email.send, recipients visible IN the frozen payload - the gate derives scopes from these bytes and nothing else.

```json
{
  "seq": 110,
  "run_id": "run_0042",
  "event_id": "f6657b75a24db02a6cd1b689af63fe5591c751ed37801bb1d06141b2cd294785",
  "prev_hash": "17a619f0b434122e5b2ccf31f9e6ee6357b80ad24326532f59bdd33a73aeba6b",
  "ts": 1751790005,
  "mono": 20,
  "principal": "agent:research",
  "type": "effect.proposed",
  "type_version": 1,
  "payload": {
    "capability": "email",
    "operation": "send",
    "input": {
      "recipients": [
        "boss@corp.example"
      ],
      "body_blob": "8cc41dbb1fe25234bd7d537c742ec4ee3daec0b4c259d86700be365cee023850"
    }
  },
  "blobs": [
    "8cc41dbb1fe25234bd7d537c742ec4ee3daec0b4c259d86700be365cee023850"
  ],
  "sig": null
}
```

### seq 111 - `gate.decision`

No matching rule: D8 default-ask. Silence is not consent.

```json
{
  "seq": 111,
  "run_id": "run_0042",
  "event_id": "132cfd77eaf45b0ccfcb9ff1979abb721503b97d8a53131d6321890de47ffe84",
  "prev_hash": "f6657b75a24db02a6cd1b689af63fe5591c751ed37801bb1d06141b2cd294785",
  "ts": 1751790005,
  "mono": 21,
  "principal": "service:kernel",
  "type": "gate.decision",
  "type_version": 1,
  "payload": {
    "effect_seq": 110,
    "action": "ask",
    "rule_id": "default-ask",
    "derived_scopes": [
      {
        "capability": "email",
        "operation": "send",
        "qualifiers": {
          "to": "boss@corp.example",
          "domain": "corp.example"
        }
      }
    ],
    "candidates": []
  },
  "blobs": [],
  "sig": null
}
```

### seq 112 - `approval.requested`

The approval inbox is a fold: requested-without-resolved. Any client can render this.

```json
{
  "seq": 112,
  "run_id": "run_0042",
  "event_id": "92f10198757d69db993bca0939690ee15cf5ae69a8c8e4c864ad187b68906f1e",
  "prev_hash": "132cfd77eaf45b0ccfcb9ff1979abb721503b97d8a53131d6321890de47ffe84",
  "ts": 1751790005,
  "mono": 22,
  "principal": "service:kernel",
  "type": "approval.requested",
  "type_version": 1,
  "payload": {
    "effect_seq": 110,
    "scopes": [
      {
        "capability": "email",
        "operation": "send",
        "qualifiers": {
          "to": "boss@corp.example"
        }
      }
    ],
    "expiry": 1751876405
  },
  "blobs": [],
  "sig": null
}
```

### seq 113 - `run.suspended`

The run goes cold - no thread, no memory, a log position. It survives reboot (roadmap E4).

```json
{
  "seq": 113,
  "run_id": "run_0042",
  "event_id": "0fb126d3d4b2e23eb58f4829ce538eda0ac92f5aafe931f30bb43a3f76c11dfc",
  "prev_hash": "92f10198757d69db993bca0939690ee15cf5ae69a8c8e4c864ad187b68906f1e",
  "ts": 1751790005,
  "mono": 23,
  "principal": "service:kernel",
  "type": "run.suspended",
  "type_version": 1,
  "payload": {
    "reason": "awaiting_approval"
  },
  "blobs": [],
  "sig": null
}
```

### seq 114 - `approval.resolved`

Signed consent (RFC-0002 s5), bound to the frozen payload of seq 110 - not to a description of it, and not portable to forks. sig is zeroed for hashing, so the illustrative value does not affect this trace's chain.

```json
{
  "seq": 114,
  "run_id": "run_0042",
  "event_id": "f28db3816cb14bae173e50dfa0f73359701fac6014440d047a851ac3770cf6ba",
  "prev_hash": "0fb126d3d4b2e23eb58f4829ce538eda0ac92f5aafe931f30bb43a3f76c11dfc",
  "ts": 1751793605,
  "mono": 24,
  "principal": "owner",
  "type": "approval.resolved",
  "type_version": 1,
  "payload": {
    "request_seq": 112,
    "decision": "approve_once",
    "bound_effect_seq": 110
  },
  "blobs": [],
  "sig": {
    "alg": "ed25519",
    "key_id": "owner-k1",
    "value": "ILLUSTRATIVE"
  }
}
```

### seq 115 - `run.resumed`

suspended -> running. Resume replays nothing; the loop continues from the log position.

```json
{
  "seq": 115,
  "run_id": "run_0042",
  "event_id": "0d4fd8c69c062b2ef81a3bb02e6bc0a2516e463d351b88dfba218a52d74adbeb",
  "prev_hash": "f28db3816cb14bae173e50dfa0f73359701fac6014440d047a851ac3770cf6ba",
  "ts": 1751793605,
  "mono": 25,
  "principal": "service:kernel",
  "type": "run.resumed",
  "type_version": 1,
  "payload": {},
  "blobs": [],
  "sig": null
}
```

### seq 116 - `effect.executed`

The broker executes the seq-110 bytes exactly. Credentials for the smtp vault slot were injected into the worker here - they appear nowhere in the log.

```json
{
  "seq": 116,
  "run_id": "run_0042",
  "event_id": "8355c2d9b66c25df6101c212010dd8573e490a835045b045da66f4f8c47d00ba",
  "prev_hash": "0d4fd8c69c062b2ef81a3bb02e6bc0a2516e463d351b88dfba218a52d74adbeb",
  "ts": 1751793605,
  "mono": 26,
  "principal": "service:broker",
  "type": "effect.executed",
  "type_version": 1,
  "payload": {
    "effect_seq": 110,
    "idempotency_key": "idem-110",
    "isolation": {
      "platform": "linux",
      "mechanisms": [
        "landlock",
        "seccomp",
        "netns"
      ]
    }
  },
  "blobs": [],
  "sig": null
}
```

### seq 117 - `effect.result`

Result recorded; the crash window between seq 116 and 117 is what D13 governs - email.send is suspend-on-uncertain.

```json
{
  "seq": 117,
  "run_id": "run_0042",
  "event_id": "5c26c7854b5e41e9c4c116e2f2b17feafa8824915c45f57ade0991acfa70a651",
  "prev_hash": "8355c2d9b66c25df6101c212010dd8573e490a835045b045da66f4f8c47d00ba",
  "ts": 1751793607,
  "mono": 27,
  "principal": "service:broker",
  "type": "effect.result",
  "type_version": 1,
  "payload": {
    "effect_seq": 110,
    "output_blob": "22ae71c8fe26920d5d372470eef863ad4f4805984770c2125747ee798410a944",
    "taint": {
      "source": "capability:email"
    }
  },
  "blobs": [
    "22ae71c8fe26920d5d372470eef863ad4f4805984770c2125747ee798410a944"
  ],
  "sig": null
}
```

### seq 118 - `msg.appended`

Invariant I2 satisfied: t1 answered exactly once, as a block in a user-role message (D1).

```json
{
  "seq": 118,
  "run_id": "run_0042",
  "event_id": "8e1f5d3ce43afab20cfde0144d352ec28c1ed4db8e2998c320bc2798d45d9ea2",
  "prev_hash": "5c26c7854b5e41e9c4c116e2f2b17feafa8824915c45f57ade0991acfa70a651",
  "ts": 1751793607,
  "mono": 28,
  "principal": "agent:research",
  "type": "msg.appended",
  "type_version": 1,
  "payload": {
    "message_id": "m3",
    "role": "user",
    "blocks": [
      {
        "id": "b1",
        "type": "core.tool_result",
        "tool_use_id": "t1",
        "output_blob": "22ae71c8fe26920d5d372470eef863ad4f4805984770c2125747ee798410a944"
      }
    ]
  },
  "blobs": [
    "22ae71c8fe26920d5d372470eef863ad4f4805984770c2125747ee798410a944"
  ],
  "sig": null
}
```

### seq 119 - `msg.appended`

Final assistant message.

```json
{
  "seq": 119,
  "run_id": "run_0042",
  "event_id": "7bc62667afb6455b9f0610b9c63a54f590d13aac92919cdb36ae555b584926d9",
  "prev_hash": "8e1f5d3ce43afab20cfde0144d352ec28c1ed4db8e2998c320bc2798d45d9ea2",
  "ts": 1751793608,
  "mono": 29,
  "principal": "agent:research",
  "type": "msg.appended",
  "type_version": 1,
  "payload": {
    "message_id": "m4",
    "role": "assistant",
    "blocks": [
      {
        "id": "b1",
        "type": "core.text",
        "body_blob": "15c52ec9e6b40235ac30170fe61438d957475de8b1bddd6a8a2500ef904f24a4"
      }
    ]
  },
  "blobs": [
    "15c52ec9e6b40235ac30170fe61438d957475de8b1bddd6a8a2500ef904f24a4"
  ],
  "sig": null
}
```

### seq 120 - `run.completed`

Terminal state. The run may now be compacted: snapshots hot, raw events cold, chain intact.

```json
{
  "seq": 120,
  "run_id": "run_0042",
  "event_id": "36283240ba955ea123c4eb7fe941f07106c027890c019d26d521c50e9529b59a",
  "prev_hash": "7bc62667afb6455b9f0610b9c63a54f590d13aac92919cdb36ae555b584926d9",
  "ts": 1751793608,
  "mono": 30,
  "principal": "service:kernel",
  "type": "run.completed",
  "type_version": 1,
  "payload": {},
  "blobs": [],
  "sig": null
}
```

**Chain head after seq 120:** `36283240ba955ea123c4eb7fe941f07106c027890c019d26d521c50e9529b59a`

A background reducer re-derives this head continuously; the anchor event exports it (RFC-0002 D4).