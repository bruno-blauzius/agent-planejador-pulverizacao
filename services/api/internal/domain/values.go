package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxEmailLength = 254
	maxNameLength  = 200
)

// Email is a normalized (trimmed, lower case) and syntactically valid address.
type Email string

// ParseEmail validates and normalizes an e-mail address.
func ParseEmail(value string) (Email, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	at := strings.LastIndex(normalized, "@")
	if normalized == "" || len(normalized) > maxEmailLength || at <= 0 || at == len(normalized)-1 ||
		strings.ContainsFunc(normalized, unicode.IsSpace) || strings.Count(normalized, "@") != 1 {
		return "", Invalid("email", "e-mail inválido")
	}
	host := normalized[at+1:]
	if !strings.Contains(host, ".") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return "", Invalid("email", "e-mail inválido")
	}
	return Email(normalized), nil
}

// String returns the normalized address.
func (e Email) String() string { return string(e) }

// ParseName validates a display name and returns it trimmed.
func ParseName(field, value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", Invalid(field, "informe o nome")
	}
	if utf8.RuneCountInString(name) > maxNameLength {
		return "", Invalid(field, "nome muito longo")
	}
	return name, nil
}

// Document is a CNPJ with a valid check digit, stored as 14 digits.
type Document string

// ParseDocument accepts a CNPJ with or without mask.
func ParseDocument(value string) (Document, error) {
	digits := digitsOnly(value)
	if len(digits) != 14 || !validCNPJ(digits) {
		return "", Invalid("document", "CNPJ inválido")
	}
	return Document(digits), nil
}

func digitsOnly(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func validCNPJ(digits string) bool {
	if strings.Count(digits, digits[:1]) == len(digits) {
		return false
	}
	return cnpjDigit(digits[:12]) == int(digits[12]-'0') && cnpjDigit(digits[:13]) == int(digits[13]-'0')
}

func cnpjDigit(prefix string) int {
	weight := 2
	sum := 0
	for i := len(prefix) - 1; i >= 0; i-- {
		sum += int(prefix[i]-'0') * weight
		weight++
		if weight > 9 {
			weight = 2
		}
	}
	remainder := sum % 11
	if remainder < 2 {
		return 0
	}
	return 11 - remainder
}
