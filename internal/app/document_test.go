package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/app/ports"
)

const sha = "9987abe2fa94c01c9fe7a386466989ffa342ffd6425f00dcb3e11f22c56f8587"

func culCtx() *ports.DocContext {
	return &ports.DocContext{ID: "d", TypeCode: "CUL", Verification: "AUTO_CUL", TenantRUC: "20608337602", TalentDocType: "DNI", FileSHA256: sha}
}

// Antes de consultar: falta el RUC, el código o el documento, o el CUL es de otra persona.
func TestCULRequest(t *testing.T) {
	noRUC := culCtx()
	noRUC.TenantRUC = ""
	cases := []struct {
		name      string
		dc        *ports.DocContext
		talentDoc string
		hint      CULHint
		want      string // estado:motivo, o "consulta:<doc>"
	}{
		{"sin RUC del tenant", noRUC, "43989632", CULHint{"20269409807", "43989632"}, "PENDING:TENANT_RUC_MISSING"},
		{"QR sin código", culCtx(), "43989632", CULHint{"", "43989632"}, "PENDING:CUL_CODE_MISSING"},
		{"sin documento en QR ni talento", culCtx(), "", CULHint{"20269409807", ""}, "PENDING:CUL_DOC_MISSING"},
		{"CUL de otra persona", culCtx(), "12345678", CULHint{"20269409807", "43989632"}, "REJECTED:CUL_OTHER_PERSON"},
		{"coincide", culCtx(), "43989632", CULHint{"20269409807", "43989632"}, "consulta:43989632"},
		{"talento sin documento: usa el del QR", culCtx(), "", CULHint{"20269409807", "43989632"}, "consulta:43989632"},
	}
	for _, c := range cases {
		_, doc, v := culRequest(c.dc, c.talentDoc, c.hint)
		got := "consulta:" + doc
		if v != nil {
			got = v.Status + ":" + v.Reason
		}
		if got != c.want {
			t.Errorf("%s: %s, se esperaba %s", c.name, got, c.want)
		}
	}
}

// Auténtico solo si existe, el PDF es idéntico al oficial y está vigente; la vigencia sale de la emisión real.
func TestCULVerdict(t *testing.T) {
	issued := time.Date(2026, 8, 20, 15, 40, 34, 0, time.UTC)
	ok := &ports.CULCheck{Found: true, Vigente: true, Names: "CRIS ALAN BONIFAZ MENACHO", IssuedAt: issued, OfficialSHA256: strings.ToUpper(sha)}
	altered := *ok
	altered.OfficialSHA256 = "00"
	expired := *ok
	expired.Vigente = false
	cases := []struct {
		name string
		chk  *ports.CULCheck
		err  error
		want string
	}{
		{"auténtico", ok, nil, "VERIFIED:"},
		{"archivo alterado", &altered, nil, "REJECTED:CUL_FILE_ALTERED"},
		{"no vigente", &expired, nil, "REJECTED:CUL_EXPIRED"},
		{"no existe", &ports.CULCheck{Found: false, Message: "No existe"}, nil, "REJECTED:CUL_NOT_FOUND"},
		{"MTPE caído", nil, errors.New("timeout"), "PENDING:CUL_UNREACHABLE"},
		{"tipo no admitido", nil, ports.ErrCULDocType, "PENDING:CUL_DOC_TYPE"},
	}
	for _, c := range cases {
		v := culVerdict(culCtx(), "20269409807", "43989632", c.chk, c.err, issued)
		if got := v.Status + ":" + v.Reason; got != c.want || v.Method != "AUTO" {
			t.Errorf("%s: %s/%s, se esperaba %s", c.name, got, v.Method, c.want)
		}
	}
	v := culVerdict(culCtx(), "20269409807", "43989632", ok, nil, issued)
	if v.ExpiresAt == nil || v.ExpiresAt.Format("2006-01-02") != "2026-11-18" {
		t.Errorf("vence a los 90 días de la emisión: %v", v.ExpiresAt)
	}
	var res map[string]any
	if json.Unmarshal(v.Result, &res) != nil || res["match"] != true || res["names"] != "CRIS ALAN BONIFAZ MENACHO" {
		t.Errorf("resultado: %s", v.Result)
	}
}

type plainCipher struct{ ports.Cipher }

func (plainCipher) Decrypt(b []byte) (string, error) { return string(b), nil }

// Los nombres cifrados (*NameEnc) se reemplazan por *Name descifrado a cualquier profundidad.
func TestDecryptNames(t *testing.T) {
	enc := base64.StdEncoding.EncodeToString([]byte("Ana Ríos"))
	in := `{"events":[{"type":"ADDED","byNameEnc":"` + enc + `"}],"documents":[{"current":{"verifiedByNameEnc":"` + enc + `"}}]}`
	out := string((&DocumentService{Cipher: plainCipher{}}).decryptNames(json.RawMessage(in)))
	if strings.Contains(out, "NameEnc") || strings.Count(out, `"Ana Ríos"`) != 2 || !strings.Contains(out, `"byName"`) || !strings.Contains(out, `"verifiedByName"`) {
		t.Errorf("salida: %s", out)
	}
}
