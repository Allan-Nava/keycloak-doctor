package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Allan-Nava/keycloak-doctor/internal/engine"
)

// ReadBaseline reads the findings of a previous run from the JSON this package
// writes, so `--output json --out-file audit.json` on one run is the baseline of
// the next. It lives here because this package owns that shape: nothing else has
// to know what the file looks like.
//
// Only the findings are read. The rest of the file describes a run that is over —
// its duration, its source, the version that produced it — and comparing those
// would make an unrelated change look like a regression.
func ReadBaseline(r io.Reader) ([]engine.Finding, error) {
	var file struct {
		Findings []engine.Finding `json:"findings"`
	}
	dec := json.NewDecoder(r)
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("reading the baseline: %w (it is the file written by --output json)", err)
	}
	if file.Findings == nil {
		return nil, fmt.Errorf("the baseline has no \"findings\": it is not the output of --output json")
	}
	// A baseline is a record of a past run: whether those findings were new *then*
	// says nothing about now, and carrying the flag over would mark them new again.
	for i := range file.Findings {
		file.Findings[i].New = false
	}
	return file.Findings, nil
}
