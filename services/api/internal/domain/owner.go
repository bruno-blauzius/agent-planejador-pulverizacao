package domain

import (
	"strings"
	"time"
)

// PersonType says whether an owner is a natural person (CPF) or a legal entity (CNPJ).
type PersonType string

// Person types.
const (
	PersonIndividual PersonType = "individual"
	PersonCompany    PersonType = "company"
)

// Valid reports whether t is a known person type.
func (t PersonType) Valid() bool { return t == PersonIndividual || t == PersonCompany }

// TaxID is a CPF (individual, 11 digits) or a CNPJ (company, 14 digits) with
// valid check digits, stored without mask.
type TaxID string

// ParseTaxID accepts a CPF or CNPJ with or without mask, according to the person type.
func ParseTaxID(personType PersonType, value string) (TaxID, error) {
	digits := digitsOnly(value)
	switch personType {
	case PersonIndividual:
		if digits == "" {
			return "", Invalid("document", "informe o CPF")
		}
		if len(digits) != 11 || !validCPF(digits) {
			return "", Invalid("document", "CPF inválido")
		}
	case PersonCompany:
		if digits == "" {
			return "", Invalid("document", "informe o CNPJ")
		}
		if len(digits) != 14 || !validCNPJ(digits) {
			return "", Invalid("document", "CNPJ inválido")
		}
	default:
		return "", Invalid("person_type", "tipo de pessoa desconhecido")
	}
	return TaxID(digits), nil
}

func validCPF(digits string) bool {
	if strings.Count(digits, digits[:1]) == len(digits) {
		return false
	}
	return cpfDigit(digits[:9]) == int(digits[9]-'0') && cpfDigit(digits[:10]) == int(digits[10]-'0')
}

func cpfDigit(prefix string) int {
	sum := 0
	weight := len(prefix) + 1
	for i := range len(prefix) {
		sum += int(prefix[i]-'0') * weight
		weight--
	}
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}

// Phone is a Brazilian phone number with area code, digits only (10 or 11 digits).
type Phone string

// ParsePhone accepts a phone with or without mask.
func ParsePhone(value string) (Phone, error) {
	digits := digitsOnly(value)
	if len(digits) < 10 || len(digits) > 11 {
		return "", Invalid("phone", "telefone inválido")
	}
	return Phone(digits), nil
}

// OwnerDetails are the editable fields of an owner, as received from the client.
// Blank optional text means "not informed".
type OwnerDetails struct {
	PersonType          PersonType
	Name                string
	Document            string
	BirthDate           *time.Time
	Phone               string
	Email               string
	WindowAlertsEnabled bool
}

// Owner is a person or company linked to farms of a company (owner, tenant or
// legal representative). The document identifies it inside the company.
type Owner struct {
	ID                  OwnerID
	CompanyID           CompanyID
	PersonType          PersonType
	Name                string
	Document            TaxID
	BirthDate           *time.Time
	Phone               *Phone
	Email               *Email
	WindowAlertsEnabled bool
	// DocumentsPending is true while the owner lacks the document the person
	// type requires (see RequiredDocumentsMissing); it is kept in step with the
	// documents by the use cases.
	DocumentsPending bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Version          int
}

// NewOwner validates the details and returns a new owner of the company.
func NewOwner(id OwnerID, companyID CompanyID, in OwnerDetails, now time.Time) (Owner, error) {
	owner := Owner{ID: id, CompanyID: companyID, DocumentsPending: true, CreatedAt: now, UpdatedAt: now}
	if err := owner.apply(in, now, false); err != nil {
		return Owner{}, err
	}
	return owner, nil
}

// Revise replaces the editable fields. The person type and the document
// identify the owner and cannot change.
func (o *Owner) Revise(in OwnerDetails, now time.Time) error {
	updated := *o
	if err := updated.apply(in, now, true); err != nil {
		return err
	}
	updated.UpdatedAt = now
	*o = updated
	return nil
}

func (o *Owner) apply(in OwnerDetails, now time.Time, existing bool) error {
	var problems Violations
	if !in.PersonType.Valid() {
		return Invalid("person_type", "tipo de pessoa desconhecido")
	}
	name, err := ParseName("name", in.Name)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return mergeErr
	}
	document, err := ParseTaxID(in.PersonType, in.Document)
	if mergeErr := problems.Merge(err); mergeErr != nil {
		return mergeErr
	}
	if existing && err == nil && (document != o.Document || in.PersonType != o.PersonType) {
		problems.Add("document", "o CPF/CNPJ não pode ser alterado")
	}
	birthDate := ownerBirthDate(&problems, in, now)
	phone := optionalPhone(&problems, in.Phone)
	email := optionalEmail(&problems, in.Email)
	if in.WindowAlertsEnabled && phone == nil && strings.TrimSpace(in.Phone) == "" {
		problems.Add("phone", "informe o WhatsApp para receber alertas de janela")
	}
	if err := problems.Err(); err != nil {
		return err
	}

	o.PersonType = in.PersonType
	o.Name = name
	o.Document = document
	o.BirthDate = birthDate
	o.Phone = phone
	o.Email = email
	o.WindowAlertsEnabled = in.WindowAlertsEnabled
	return nil
}

func ownerBirthDate(problems *Violations, in OwnerDetails, now time.Time) *time.Time {
	if in.BirthDate == nil {
		return nil
	}
	if in.PersonType != PersonIndividual {
		problems.Add("birth_date", "data de nascimento só se aplica a pessoa física")
		return nil
	}
	date := time.Date(in.BirthDate.Year(), in.BirthDate.Month(), in.BirthDate.Day(), 0, 0, 0, 0, time.UTC)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if date.After(today) {
		problems.Add("birth_date", "a data não pode estar no futuro")
		return nil
	}
	return &date
}

func optionalPhone(problems *Violations, value string) *Phone {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	phone, err := ParsePhone(value)
	if err != nil {
		problems.Add("phone", "telefone inválido")
		return nil
	}
	return &phone
}

func optionalEmail(problems *Violations, value string) *Email {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	email, err := ParseEmail(value)
	if err != nil {
		problems.Add("email", "e-mail inválido")
		return nil
	}
	return &email
}
