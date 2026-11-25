package app

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

// Snapshot de ingreso: los solicitantes (quien confirmó y el analista del RQ) llegan descifrados y planos; sin analista, null.
func TestDecryptHireDoc(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	in := `{"id":"c1","talent":{"firstName":"Ana","emailEnc":"` + b64("ana@x.pe") + `","docNumberEnc":null,"phoneEnc":""},` +
		`"confirmedByNameEnc":"` + b64("Luis Pérez") + `","confirmedByEmailEnc":"` + b64("luis@x.pe") + `",` +
		`"confirmedAt":"2026-11-25T10:00:00.000000Z","rqAnalystNameEnc":null,"rqAnalystEmailEnc":null}`
	var m map[string]any
	if err := json.Unmarshal(decryptHireDoc(plainCipher{}, json.RawMessage(in)), &m); err != nil {
		t.Fatal(err)
	}
	if m["confirmedByName"] != "Luis Pérez" || m["confirmedByEmail"] != "luis@x.pe" || m["confirmedAt"] != "2026-11-25T10:00:00.000000Z" {
		t.Fatalf("confirmado por: %v", m)
	}
	for _, k := range []string{"rqAnalystName", "rqAnalystEmail"} {
		if v, ok := m[k]; !ok || v != nil {
			t.Fatalf("%s debe ser null: %v", k, m)
		}
	}
	for _, k := range []string{"confirmedByNameEnc", "confirmedByEmailEnc", "rqAnalystNameEnc", "rqAnalystEmailEnc"} {
		if _, ok := m[k]; ok {
			t.Fatalf("queda %s cifrado: %v", k, m)
		}
	}
	tl := m["talent"].(map[string]any)
	if tl["email"] != "ana@x.pe" || tl["docNumber"] != "" || tl["phone"] != "" {
		t.Fatalf("talento: %v", tl)
	}
	in = `{"rqAnalystNameEnc":"` + b64("Rosa") + `","rqAnalystEmailEnc":"` + b64("rosa@x.pe") + `"}`
	m = nil
	_ = json.Unmarshal(decryptHireDoc(plainCipher{}, json.RawMessage(in)), &m)
	if m["rqAnalystName"] != "Rosa" || m["rqAnalystEmail"] != "rosa@x.pe" {
		t.Fatalf("analista: %v", m)
	}
}
