// View failure values (WP-05a). Codes come from docs/errors.md
// VERBATIM — never invent ad-hoc error strings (errors.md law).
package fold

import (
	"fmt"

	"github.com/DonScott603/Agent-Runtime/kernel"
)

// Error codes recorded on failed view instances (docs/errors.md).
//
// PLUGIN_CONTRACT on the read path: errors.md maps it to run.failed,
// but a reducer panic during a rebuild has no running run to fail —
// the fold surfaces a view-unavailable error carrying the code
// (CHAIN_BROKEN's row sets the read-path precedent: "none (read
// path)"). Wiring to run.failed on the live path is WP-07+.
const (
	CodeSchemaUnknownVersion = "SCHEMA_UNKNOWN_VERSION" // type_version above declared support; view unavailable
	CodePluginContract       = "PLUGIN_CONTRACT"        // reducer panic, or nil state with nil error
	CodePluginError          = "PLUGIN_ERROR"           // reducer returned a deterministic error VALUE
)

// ViewError is the sticky failure of one view instance. It is an
// error VALUE, not hashed state: Detail may carry diagnostic text
// (e.g. the panic value), but determinism comparisons use Code and
// state hashes only (owner A1: strings entering comparable output
// are built from stable structured facts).
type ViewError struct {
	View   string
	Run    kernel.RunID // "" for owner-scoped views
	Seq    kernel.Seq   // seq of the event at which the instance failed
	Code   string
	Detail string
	Err    error // underlying reducer error for PLUGIN_ERROR, else nil
}

func (e *ViewError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: view %q run %q seq %d: %s: %v", e.Code, e.View, e.Run, e.Seq, e.Detail, e.Err)
	}
	return fmt.Sprintf("%s: view %q run %q seq %d: %s", e.Code, e.View, e.Run, e.Seq, e.Detail)
}

func (e *ViewError) Unwrap() error { return e.Err }
