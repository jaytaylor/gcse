package gcse

import (
	"github.com/daviddengcn/sophie"
	"github.com/golangplus/errors"
)

const (
	NDA_UPDATE   = iota // Indicates the whole document has been updated.
	NDA_STARS           // Indicates only stars are updated.
	NDA_DEL             // Indicates deleted.
	NDA_ORIGINAL        // Indicates original document.
)

// NewDocAction is used while merging docs.
//
// Note: If Action equals NDA_DEL, DocInfo is undefined.
type NewDocAction struct {
	DocInfo

	Action sophie.VInt
}

// NewNewDocAction returns a new instance of *NewDocAction as a Sophier.
func NewNewDocAction() sophie.Sophier {
	nda := &NewDocAction{}
	return nda
}

func (nda *NewDocAction) WriteTo(w sophie.Writer) error {
	if err := nda.Action.WriteTo(w); err != nil {
		return err
	}
	if nda.Action == NDA_DEL {
		return nil
	}
	return nda.DocInfo.WriteTo(w)
}

func (nda *NewDocAction) ReadFrom(r sophie.Reader, l int) error {
	if err := nda.Action.ReadFrom(r, -1); err != nil {
		return errorsp.WithStacks(err)
	}
	if nda.Action == NDA_DEL {
		return nil
	}
	return errorsp.WithStacks(nda.DocInfo.ReadFrom(r, -1))
}
