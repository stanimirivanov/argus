package change

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/stanimirivanov/argus/internal/catalog"
)

var fieldSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var fieldNamePattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,254}$`)

// FieldRename is immutable OpenAPI contract proof for a single declared JSON
// property rename. It carries no permission to patch or publish test source.
type FieldRename struct {
	DocumentPath string   `json:"documentPath"`
	BaseSHA256   string   `json:"baseSha256"`
	HeadSHA256   string   `json:"headSha256"`
	Method       string   `json:"method"`
	Path         string   `json:"path"`
	OperationID  string   `json:"operationId"`
	Capabilities []string `json:"capabilities"`
	Direction    string   `json:"direction"`
	StatusCode   string   `json:"statusCode"`
	PreviousName string   `json:"previousName"`
	Name         string   `json:"name"`
}

// ValidateFieldRename checks portable evidence shape. The analyzer establishes
// semantics by comparing both source documents at immutable revisions.
func ValidateFieldRename(rename FieldRename) error {
	if !validRepositoryPath(rename.DocumentPath) || !fieldSHA256Pattern.MatchString(rename.BaseSHA256) ||
		!fieldSHA256Pattern.MatchString(rename.HeadSHA256) || !fieldNamePattern.MatchString(rename.PreviousName) ||
		!fieldNamePattern.MatchString(rename.Name) || rename.PreviousName == rename.Name ||
		!validFieldMethod(rename.Method) || !strings.HasPrefix(rename.Path, "/") || len(rename.Path) > 2048 ||
		strings.TrimSpace(rename.OperationID) == "" || len(rename.OperationID) > 255 ||
		len(rename.Capabilities) == 0 || len(rename.Capabilities) > MaxOperationCapabilities ||
		!slices.Equal(rename.Capabilities, sortedUnique(rename.Capabilities)) ||
		(rename.Direction != "request" && rename.Direction != "response") ||
		(rename.Direction == "request" && rename.StatusCode != "") ||
		(rename.Direction == "response" && !validResponseStatus(rename.StatusCode)) {
		return fmt.Errorf("%w: field rename proof", ErrInvalid)
	}
	for _, capability := range rename.Capabilities {
		if !catalog.IsLocalKey(capability) {
			return fmt.Errorf("%w: field rename capability", ErrInvalid)
		}
	}

	return nil
}

func validFieldMethod(method string) bool {
	switch method {
	case "GET", "PUT", "POST", "DELETE", "PATCH":
		return true
	default:
		return false
	}
}

func validResponseStatus(status string) bool {
	return len(status) == 3 && status[0] >= '1' && status[0] <= '5' &&
		status[1] >= '0' && status[1] <= '9' && status[2] >= '0' && status[2] <= '9'
}
