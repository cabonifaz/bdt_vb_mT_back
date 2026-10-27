package domain

// PersonIdentity: datos de una persona obtenidos de un registro público (RENIEC) por su número de documento.
type PersonIdentity struct {
	DocumentNumber string  `json:"documentNumber"`
	FirstName      string  `json:"firstName"`
	LastName       string  `json:"lastName"`
	SecondLastName string  `json:"secondLastName"`
	BirthDate      *string `json:"birthDate,omitempty"` // YYYY-MM-DD, solo si el proveedor la entrega
	Provider       string  `json:"provider"`
}
