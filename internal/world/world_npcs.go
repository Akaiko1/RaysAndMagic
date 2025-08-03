package world

import (
	"ugataima/internal/character"
)

// placeSkillTeachers places NPCs that can teach skills throughout the world
func (w *World3D) placeSkillTeachers() {
	// Sword Master near clearings
	swordMaster := character.NewSkillTeacher(
		"Master Gareth",
		character.SkillSword,
		character.MasteryGrandMaster,
		20.0*64, 15.0*64,
	)
	w.Teachers = append(w.Teachers, swordMaster)

	// Magic Teacher near ancient tree
	magicTeacher := character.NewSkillTeacher(
		"Archmage Lysander",
		character.MagicFire,
		character.MasteryMaster,
		35.0*64, 25.0*64,
	)
	w.Teachers = append(w.Teachers, magicTeacher)

	// Body Magic Healer near water
	healer := character.NewSkillTeacher(
		"Priestess Celestine",
		character.MagicBody,
		character.MasteryExpert,
		10.0*64, 30.0*64,
	)
	w.Teachers = append(w.Teachers, healer)

	// Archery Instructor in clearing
	archeryInstructor := character.NewSkillTeacher(
		"Ranger Silvelyn",
		character.SkillBow,
		character.MasteryExpert,
		40.0*64, 10.0*64,
	)
	w.Teachers = append(w.Teachers, archeryInstructor)
}
