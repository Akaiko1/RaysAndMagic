package game

import "ugataima/internal/shaders"

// Runtime compilation and shadergen read the same embedded Kage sources.
var (
	fireflyShaderSrc           = shaders.Source("firefly.kage")
	worldMaterialShaderSrc     = shaders.Source("world_material.kage")
	floorShaderSrc             = shaders.Source("floor.kage")
	turnBlurShaderSrc          = shaders.Source("turn_blur.kage")
	skyShaderSrc               = shaders.Source("sky.kage")
	campDissolveShaderSrc      = shaders.Source("camp_dissolve.kage")
	crystalShimmerShaderSource = shaders.Source("crystal_shimmer.kage")
	standeeTrilinearShaderSrc  = shaders.Source("standee_trilinear.kage")
	standeeVolumeShaderSrc     = shaders.Source("standee_volume.kage")
	weaponRibbonShaderSrc      = shaders.Source("weapon_ribbon.kage")
	impactMaterialShaderSrc    = shaders.Source("impact_material.kage")
	weaponOrbShaderSrc         = shaders.Source("weapon_orb.kage")
	spellBoltShaderSrc         = shaders.Source("spell_bolt.kage")
	spellBodyShaderSrc         = shaders.Source("spell_body.kage")
	zonePlumeShaderSrc         = shaders.Source("zone_plume.kage")
	weaponBodyShaderSrc        = shaders.Source("weapon_body.kage")
	bubbleShaderSrc            = shaders.Source("bubble.kage")
	auraCurtainShaderSrc       = shaders.Source("aura_curtain.kage")
)
