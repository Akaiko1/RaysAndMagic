package items

// WeaponSignatureState travels with the physical weapon through transfers,
// equipment, stash and saves. A projectile holds a transient reservation on
// launch; only its landed hit spends the saved charge on that instance.
type WeaponSignatureState struct {
	Charges      int     `json:"charges,omitempty"`
	MoveDistance float64 `json:"move_distance,omitempty"` // tiles moved toward the next charge
}
