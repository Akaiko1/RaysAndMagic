package character

type SkillType int

const (
	// Weapon skills
	SkillSword SkillType = iota
	SkillDagger
	SkillAxe
	SkillSpear
	SkillBow
	SkillMace
	SkillStaff

	// Armor skills
	SkillLeather
	SkillChain
	SkillPlate
	SkillShield

	// Misc skills
	SkillBodybuilding
	SkillMeditation
	SkillMerchant
	SkillRepair
	SkillIdentifyItem
	SkillDisarmTrap
	SkillLearning
	SkillArmsMaster
)

type SkillMastery int

const (
	MasteryNovice SkillMastery = iota
	MasteryExpert
	MasteryMaster
	MasteryGrandMaster
)

type Skill struct {
	Level   int
	Mastery SkillMastery
}
