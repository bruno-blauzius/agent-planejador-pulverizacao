package domain

import (
	"fmt"
	"math"
	"strings"
)

// LinkRole is how an owner relates to a farm.
type LinkRole string

// Link roles. Only LinkOwner holds a share of the farm.
const (
	LinkOwner               LinkRole = "owner"
	LinkTenant              LinkRole = "tenant"
	LinkLegalRepresentative LinkRole = "legal_representative"
)

// Valid reports whether r is a known link role.
func (r LinkRole) Valid() bool {
	return r == LinkOwner || r == LinkTenant || r == LinkLegalRepresentative
}

// SharePercent is a share of a farm in hundredths of a percent (10000 = 100%).
// Integers keep the "sums exactly 100%" rule free of floating point error.
type SharePercent int

// FullOwnership is 100% of a farm.
const FullOwnership SharePercent = 10000

// ParseSharePercent converts a percentage with up to two decimals (e.g. 33.33).
func ParseSharePercent(field string, value float64) (SharePercent, error) {
	hundredths := math.Round(value * 100)
	switch {
	case math.IsNaN(value) || math.IsInf(value, 0):
		return 0, Invalid(field, "participação inválida")
	case math.Abs(value*100-hundredths) > 1e-6:
		return 0, Invalid(field, "use no máximo duas casas decimais")
	case hundredths <= 0 || hundredths > float64(FullOwnership):
		return 0, Invalid(field, "a participação deve ser maior que 0 e no máximo 100")
	}
	return SharePercent(hundredths), nil
}

// Percent returns the share as a percentage (e.g. 33.33).
func (s SharePercent) Percent() float64 { return float64(s) / 100 }

// String formats the share for messages in Portuguese (e.g. "33,33%").
func (s SharePercent) String() string {
	return strings.Replace(fmt.Sprintf("%.2f%%", s.Percent()), ".", ",", 1)
}

// Ownership is the link between one farm and one owner.
type Ownership struct {
	FarmID  FarmID
	OwnerID OwnerID
	Role    LinkRole
	// Share is set only for LinkOwner.
	Share *SharePercent
}

// OwnershipInput is a link as received from the client.
type OwnershipInput struct {
	FarmID       FarmID
	OwnerID      OwnerID
	Role         LinkRole
	SharePercent *float64
}

// NewOwnership validates one link; field is the path of the item in the
// request (e.g. "owners[0]" or "links[2]").
func NewOwnership(field string, in OwnershipInput) (Ownership, error) {
	if !in.Role.Valid() {
		return Ownership{}, Invalid(field+".role", "papel desconhecido")
	}
	link := Ownership{FarmID: in.FarmID, OwnerID: in.OwnerID, Role: in.Role}
	if in.Role != LinkOwner {
		if in.SharePercent != nil {
			return Ownership{}, Invalid(field+".share_percent", "arrendatário e responsável legal não têm participação")
		}
		return link, nil
	}
	if in.SharePercent == nil {
		return Ownership{}, Invalid(field+".share_percent", "informe a participação")
	}
	share, err := ParseSharePercent(field+".share_percent", *in.SharePercent)
	if err != nil {
		return Ownership{}, err
	}
	link.Share = &share
	return link, nil
}

// TotalShare sums the shares of the links (tenants and representatives count zero).
func TotalShare(links []Ownership) SharePercent {
	var total SharePercent
	for _, link := range links {
		if link.Share != nil {
			total += *link.Share
		}
	}
	return total
}

// FitsInFarm reports whether adding share to the shares other owners already
// hold keeps the farm at or below 100%.
func FitsInFarm(others []Ownership, share SharePercent) bool {
	return TotalShare(others)+share <= FullOwnership
}
