// culcheck verifica un CUL contra el servicio público del MTPE, igual que la validación automática de BDT,
// sin tocar la base de datos (soporte y diagnóstico).
//
//	go run ./cmd/culcheck -ruc <RUC de la empresa> -code <código del CUL> -doc <documento> -file cul.pdf
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/cabonifaz/bdt_vb_mT_back/internal/adapters/cul"
)

func main() {
	ruc := flag.String("ruc", "", "RUC de la empresa que verifica")
	code := flag.String("code", "", "código del CUL (11 dígitos, está en el QR y arriba a la derecha)")
	doc := flag.String("doc", "", "número de documento del titular")
	typ := flag.String("type", "DNI", "tipo de documento: DNI, CE o CPP")
	file := flag.String("file", "", "PDF presentado (opcional: compara su huella con la del oficial)")
	flag.Parse()
	if *ruc == "" || *code == "" || *doc == "" {
		flag.Usage()
		os.Exit(2)
	}
	c, err := cul.New().Check(context.Background(), *ruc, *code, *typ, *doc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if !c.Found {
		fmt.Println("no encontrado:", c.Message)
		os.Exit(1)
	}
	fmt.Printf("certificado %s · %s · emitido %s · vigente=%v\nhuella oficial %s\n", c.Code, c.Names, c.IssuedAt.Format("2006-01-02 15:04"), c.Vigente, c.OfficialSHA256)
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		sum := sha256.Sum256(b)
		got := hex.EncodeToString(sum[:])
		fmt.Printf("huella archivo  %s\nidéntico al oficial: %v\n", got, strings.EqualFold(got, c.OfficialSHA256))
	}
}
