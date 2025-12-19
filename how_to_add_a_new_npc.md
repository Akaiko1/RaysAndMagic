# How to Add a New NPC to UgaTaima

This manual provides a step-by-step guide for adding new NPCs to the UgaTaima RPG game, with precise details on working vs non-working features.

## System Overview

The NPC system consists of:
- **NPC Configuration**: YAML-based NPC definitions with properties and services
- **World Placement**: Map-based NPC positioning using `>[npc:key]` syntax inline with map rows
- **Interaction System**: T key dialogue interface for player-NPC interactions  
- **Service Systems**: Different NPC types providing various services

## ✅ WORKING FEATURES

### **🔮 Spell Trading System** ✅ **FULLY IMPLEMENTED**
- NPCs can teach spells to party members for gold
- Complete dialogue interface with keyboard/mouse controls
- Spell learning verification and gold deduction
- Character spellbook integration

### **🗺️ World Placement** ✅ **FULLY IMPLEMENTED**  
- NPCs spawn from map files using `>[npc:key]` syntax inline with map rows
- Automatic loading from `assets/npcs.yaml` configuration
- Multiple map support (forest, desert, water)
- @ symbol placement for visual positioning

### **💬 Dialogue Interface** ✅ **FULLY IMPLEMENTED**
- T key interaction within 2-tile range
- Arrow key navigation for character/spell selection
- Mouse click support for selections
- Enter key confirmation for purchases

### **⚔️ Encounter System** ✅ **FULLY IMPLEMENTED**
- NPCs with `type: "encounter"` trigger combat encounters
- Custom dialogue with choice prompts (leave/attack)
- Monster spawning from encounter definitions
- First-visit-only tracking for one-time encounters
- Rewards (gold, experience) on completion

### **📊 Data Structures** ✅ **FULLY IMPLEMENTED**
- Complete NPC struct with all core properties
- YAML configuration loading and parsing
- NPC creation from configuration data

## ❌ NOT YET WORKING

### **🛒 Merchant Trading** ❌ **PLACEHOLDER ONLY**
- Inventory items defined in YAML but not implemented
- No item purchasing/selling interface
- NPCItem → Item conversion not implemented

### **📜 Quest System** ❌ **PLACEHOLDER ONLY**  
- Quest definitions exist in YAML but no quest logic
- No quest tracking or completion system
- Quest dialogue not implemented

### **🎯 Advanced Features** ❌ **PARTIALLY IMPLEMENTED**
- Custom dialogue messages ignored (hardcoded dialogue used instead)
- Spell metadata (level, spell_points, description) ignored  
- Requirements checking marked as TODO
- Placement rules defined but not enforced

## 📋 Property Usage Analysis

### **✅ REQUIRED PROPERTIES**

#### **Core NPC Properties:**
```yaml
npcs:
  my_npc:
    name: "NPC Name"        # ✅ REQUIRED - Used in dialog title
    type: "spell_trader"    # ✅ REQUIRED - Determines behavior  
    sprite: "elf_warrior"   # ✅ REQUIRED - Used for rendering
```

#### **Spell Trader Properties:**
```yaml
spells:
  my_spell:
    name: "Fireball"        # ✅ REQUIRED - Must match spell_types.go exactly
    school: "fire"          # ✅ REQUIRED - Used for spellbook organization
    cost: 500              # ✅ REQUIRED - Gold cost for purchasing
```

### **❓ OPTIONAL PROPERTIES**
```yaml
description: "A wise mage"  # ❓ OPTIONAL - Not displayed but stored
dialogue:
  greeting: "Hello!"       # ❓ OPTIONAL - Custom greeting (partially used)
```

### **❌ IGNORED PROPERTIES** 
```yaml
# These exist in YAML but are completely ignored:
level: 2                   # ❌ IGNORED - Not used anywhere
spell_points: 6            # ❌ IGNORED - Actual spell cost comes from spell_types.go
description: "Spell desc"  # ❌ IGNORED - Not shown in interface  
requirements:              # ❌ IGNORED - No requirement checking implemented
  min_level: 3
dialogue:                  # ❌ MOSTLY IGNORED - Only greeting partially used
  teaching: "Custom text"   # ❌ IGNORED - Hardcoded dialogue used instead
  insufficient_gold: "..."  # ❌ IGNORED - Hardcoded dialogue used instead
```

## Step-by-Step Implementation

### 1. Define NPC in Configuration

**File**: `assets/npcs.yaml`

**✅ MINIMAL WORKING EXAMPLE:**
```yaml
npcs:
  my_spell_mage:                    # ← Unique NPC key
    name: "Archmage Merlin"         # ✅ REQUIRED - Display name  
    type: "spell_trader"            # ✅ REQUIRED - NPC type
    sprite: "elf_warrior"           # ✅ REQUIRED - Sprite filename
    
    # ✅ REQUIRED: Spells this NPC can teach
    spells:
      my_fireball:                  # ← Unique spell key (any name)
        name: "Fireball"            # ✅ MUST MATCH spell_types.go exactly
        school: "fire"              # ✅ REQUIRED for spellbook organization  
        cost: 500                   # ✅ REQUIRED gold cost
      
      my_heal:
        name: "Heal"
        school: "body"
        cost: 300
```

**🔥 CRITICAL REQUIREMENTS:**
- **Spell Name Matching**: `name` field **MUST EXACTLY MATCH** spell names in `spell_types.go`
- **Case Sensitivity**: "Fire Bolt" ≠ "fire bolt" ≠ "Firebolt"
- **Sprite Files**: Must exist in `assets/sprites/characters/`

### 2. Place NPC in Map

**File**: `assets/forest.map` (or desert.map, water.map)

```
# Map layout with NPC placement
%.@..T...T....T...R.....T........T....T..........%  >[npc:my_spell_mage]
%.....P....T......................T..........d...%
%..T.T.......CCCCCC..T.....A.......T.............%
```

**Key Points:**
- **@ Symbol**: Visual marker for NPC position in map grid
- **>[npc:key]**: Entity definition **on same line** as map row
- **Spacing**: Entity definitions align with the right edge for readability

### 3. Verify Implementation

#### **Required Files Check:**
- [ ] `assets/npcs.yaml` contains NPC definition
- [ ] `assets/sprites/characters/[sprite].png` exists
- [ ] Map file contains `>[npc:key]` definition inline with @ symbol
- [ ] Spell names exactly match those in `internal/spells/spell_types.go`

## Quick Reference: NPC Types

| Type | Status | Features | Limitations |
|------|--------|----------|-------------|
| `spell_trader` | ✅ **Working** | Spell teaching, gold costs | Custom dialogue ignored, requirements ignored |
| `encounter` | ✅ **Working** | Combat encounters, dialogue choices, rewards | First-visit tracking only |
| `merchant` | ❌ **Placeholder** | YAML inventory defined | No trading interface |
| `quest_giver` | ❌ **Placeholder** | YAML quest defined | No quest system |
| `skill_trainer` | ✅ **Working** | Hardcoded in game logic | Not YAML configurable |

## Testing Checklist

### **✅ MINIMAL NPC VALIDATION**
- [ ] **Required Properties Only**:
  - [ ] `name` field exists
  - [ ] `type: "spell_trader"`
  - [ ] `sprite` field exists and sprite file present
  - [ ] `spells` section with at least one spell
  - [ ] Each spell has `name`, `school`, `cost`

### **✅ SPELL TRADER TESTING**
- [ ] NPC appears in correct map location
- [ ] T key interaction works within range
- [ ] Dialogue interface opens properly  
- [ ] Character selection with arrows/mouse
- [ ] Spell selection with arrows/mouse
- [ ] Gold deduction on purchase
- [ ] Spell appears in character spellbook
- [ ] "Already known" detection works
- [ ] Insufficient gold message appears

## Current Limitations

### **✅ What Actually Works**
- **Spell trader NPCs with basic functionality**
- **Gold-based spell purchasing**
- **Visual rendering and interaction**
- **Spellbook integration**
- **Encounter NPCs with combat and rewards**

### **❌ What Doesn't Work (Despite YAML Support)**
- **Custom dialogue messages** (all hardcoded)
- **Spell metadata** (level, spell_points, description ignored)
- **Requirements checking** (TODO in code)
- **Merchant trading system** (placeholder only)
- **Quest system** (placeholder only)
- **Placement rules** (defined but not enforced)

### **🔧 Hardcoded vs Configurable**
- **spell_trader**: ✅ YAML configurable (basic functionality)
- **skill_trainer**: ❌ Hardcoded NPCs only (Master Gareth, Archmage Lysander, etc.)
- **merchant/quest_giver**: ❌ Placeholder types (no implementation)

The NPC system has a solid foundation with spell trading working reliably, but many advanced features remain unimplemented despite having YAML structure defined for them.