# Soundtrack Prompts (Suno v5.5)

Copy-paste Style prompts for every map in the game, in two houses:

- **MM** - the Might and Magic VI/VII house. Chamber baroque, NOT orchestral epic:
  harpsichord ostinato, pastoral woodwind leads, strings as bedding, glockenspiel
  and bells, organ in holy places, acoustic guitar in the wilds. Short monophonic
  motifs - one singable melody voice over simple backing.
- **RO** - the Ragnarok Online / soundTeMP house. Ambient with a live melody: warm
  analog pads, soft downtempo drum loops in towns, natural acoustics in the fields,
  jazz-fusion and new-age colour, cozy mid-tempo.

Every block below is a complete prompt. Paste it into Style as-is, nothing to
assemble.

## Interface settings (do not put these in the Style text)

| control | value |
| --- | --- |
| Model | v5 or v5.5 (Style field holds ~1000 chars; v4 and older cap at 200) |
| Instrumental | ON, Lyrics field empty |
| Exclude Styles | the block below |
| Style Influence | 0.8 - 1.0 (lower and Suno drifts to default pop orchestra) |
| Weirdness | 0.1 - 0.3 (a predictable loop, not an experiment) |
| Duration | 2:00 - 2:30, then cut the loop with tmp/audiogen/make_loop.py |

Exclude Styles - paste once, this is the lever that kills "epic instrumental":

    epic, cinematic, trailer music, hybrid orchestral, blockbuster, ostinato brass,
    taiko, epic choir, wall of sound, orchestral swell, dubstep, EDM drop, supersaw,
    metal, distorted guitar, vocals, lyrics, singing, rap

Suno truncates silently past the cap, with no warning. The prompts below run 125 to
431 characters, so they all fit v5/v5.5 whole. They are ordered mood -> house ->
instruments -> tempo/mode -> production, so on an older 200-char field cut from the
RIGHT: drop the production and loop tags, then the least important instrument, and
keep `instrumental` as the last word. The part up to the mode is what carries the
sound.

## Looping

    python3 tmp/audiogen/make_loop.py suno_forest.mp3 --out assets/audio/music/forest_day --preview

One decode, one encode, sample-accurate phase lock, 40 ms cross-fade. It prints the
seam error in dB; below -20 dB is inaudible. Target 1:30 - 2:30 per loop, `music`
bus in `assets/audio.yaml`, files at `assets/audio/music/<map_key>_day.ogg` /
`_night.ogg`.

---

## forest - Elvish Forest

### MM day

    pastoral idyllic woodland exploration, late-90s CRPG field theme, chamber
    baroque ensemble, harpsichord ostinato, recorder carrying one simple singable
    motif, soft string bedding, lute, glockenspiel accents, upright bass, light
    hand percussion, 84 BPM, dorian, 1998 game-soundtrack production, intimate
    small-room mix, gentle dynamics, loopable short-form cue, no build-ups, steady
    tempo, no big ending, no ritardando, instrumental

### MM night

    hushed nocturnal woodland, late-90s CRPG night field theme, sparse chamber
    baroque, celesta lead on one simple motif, music box, breathy low flute, muted
    harpsichord, sustained low strings, sparse rim clicks, 68 BPM, aeolian, 1998
    game-soundtrack production, intimate small-room mix, loopable short-form cue,
    no build-ups, steady tempo, no big ending, instrumental

### RO day

    sunlit forest trail, late-90s Korean MMO field theme, downtempo new age, nylon
    acoustic guitar lead, warm analog pads, brushed kit with soft shuffle groove,
    fretless bass, wooden flute counter-line, light shaker, 82 BPM, dorian, lush
    but soft mix, one memorable melody over a steady groove, loopable short-form
    cue, no build-ups, steady tempo, no big ending, instrumental

### RO night

    fireflies over a dark forest, late-90s Korean MMO night theme, ambient new age,
    felt piano lead, deep warm pads, soft trip-hop drum loop, sub bass, glass bells,
    distant owl-like flute, 70 BPM, aeolian, lush but soft mix, loopable short-form
    cue, no build-ups, steady tempo, no big ending, instrumental

---

## desert - Scorching Desert

### MM day

    dusty sun-bleached desert road, late-90s CRPG field theme, chamber baroque with
    middle-eastern colour, hammered dulcimer ostinato, ney flute lead on one simple
    motif, dry strings, frame drum and tambourine, upright bass, 90 BPM, harmonic
    minor, 1998 game-soundtrack production, intimate small-room mix, loopable
    short-form cue, no build-ups, steady tempo, no big ending, instrumental

### MM night

    cold desert night under stars, late-90s CRPG night theme, sparse chamber
    ensemble, oud lead, single harpsichord notes, low sustained strings, distant
    frame drum, 72 BPM, phrygian, 1998 game-soundtrack production, intimate mix,
    gentle dynamics, loopable short-form cue, no build-ups, steady tempo, no big
    ending, instrumental

### RO day

    caravan crossing hot dunes, late-90s Korean MMO field theme, downtempo world
    fusion, oud and ney trading one memorable melody, warm analog pads, darbuka
    groove with shaker, fretless bass, 94 BPM, harmonic minor, lush but soft mix,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental

### RO night

    quiet dunes after sunset, late-90s Korean MMO night theme, ambient world new
    age, ney flute lead over deep pads, soft handpan pulse, sub bass, tiny bells,
    76 BPM, hijaz, lush but soft mix, one melody over a steady groove, loopable
    short-form cue, no build-ups, steady tempo, no big ending, instrumental

---

## water - Ocean Depths

One panorama only, so one cue per house.

### MM

    open sea and scattered small islands, late-90s CRPG travel theme, chamber
    baroque, harpsichord arpeggios, oboe lead on one simple motif, softly swelling
    strings, glockenspiel, acoustic guitar counterpoint, no percussion, 76 BPM,
    lydian, 1998 game-soundtrack production, intimate small-room mix, gentle
    dynamics, loopable short-form cue, no build-ups, steady tempo, no big ending,
    instrumental

### RO

    calm ocean crossing, late-90s Korean MMO travel theme, downtempo new age,
    electric piano lead, wide reverberant pads, soft brushed kit, fretless bass,
    pan flute, high gull-like sustains, 78 BPM, lydian, lush but soft mix, one
    memorable melody over a steady groove, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

---

## highlands - Misty Highlands

### MM day

    windy misty uplands, late-90s CRPG field theme, chamber baroque with celtic
    colour, acoustic guitar duet, tin whistle lead on one simple motif, low string
    drone, soft bodhran pulse, harpsichord accents, 80 BPM, mixolydian, 1998
    game-soundtrack production, intimate small-room mix, loopable short-form cue,
    no build-ups, steady tempo, no big ending, instrumental

### MM night

    lonely cold highland night, late-90s CRPG night theme, sparse chamber ensemble,
    solo cello lead, distant tin whistle, string drone, no percussion, 64 BPM,
    aeolian, 1998 game-soundtrack production, intimate mix, gentle dynamics,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental

### RO day

    green highland plateau, late-90s Korean MMO field theme, downtempo celtic new
    age, fiddle and tin whistle lead, warm analog pads, soft bodhran and shaker
    groove, fretless bass, hammered dulcimer, 84 BPM, mixolydian, lush but soft mix,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental

### RO night

    mist settling on the highlands, late-90s Korean MMO night theme, ambient new
    age, felt piano and bowed pad, slow soft drum loop, sub bass, whistle fragments,
    66 BPM, aeolian, lush but soft mix, one melody over a steady groove, loopable
    short-form cue, no build-ups, steady tempo, no big ending, instrumental

---

## dragon_cliffs - Dragon Cliffs

### MM day

    volcanic basalt cliffs, late-90s CRPG danger-zone theme, tense chamber baroque,
    low harpsichord ostinato, bass clarinet lead on one simple motif, tremolo
    strings, soft timpani pulse, single tolling low bell, 88 BPM, phrygian, 1998
    game-soundtrack production, intimate small-room mix, loopable short-form cue,
    no build-ups, steady tempo, no big ending, instrumental

### MM night

    dragon roost in the dark, late-90s CRPG night danger theme, sparse ominous
    chamber, organ pedal note, contrabass and low strings, single tolling bell, no
    brass, no percussion beyond a soft drum, 70 BPM, phrygian, 1998 game-soundtrack
    production, intimate mix, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

### RO day

    scorched cliff path, late-90s Korean MMO danger theme, downtempo dark world
    fusion, duduk lead, dark warm pads, tribal frame drum groove, sub bass, clean
    electric piano stabs, 90 BPM, phrygian, lush but soft mix, one melody over a
    steady groove, loopable short-form cue, no build-ups, steady tempo, no big
    ending, instrumental

### RO night

    embers and basalt at night, late-90s Korean MMO night theme, ambient dark new
    age, deep drone pads, duduk lead, slow heartbeat kick, sub bass, metallic scrape
    textures, 68 BPM, phrygian, lush but soft mix, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

---

## deep_jungle - Deep Jungle

### MM day

    humid secretive jungle, late-90s CRPG field theme, chamber baroque with tropical
    colour, marimba ostinato, piccolo lead on one simple motif, muted strings, wood
    blocks and shakers, upright bass, 96 BPM, dorian, 1998 game-soundtrack
    production, intimate small-room mix, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

### MM night

    jungle canopy at night, late-90s CRPG night theme, sparse chamber ensemble,
    kalimba lead, low flute, sustained strings, soft congas, insect-like high
    pizzicato, 74 BPM, aeolian, 1998 game-soundtrack production, intimate mix,
    gentle dynamics, loopable short-form cue, no build-ups, steady tempo, no big
    ending, instrumental

### RO day

    river deep into the jungle, late-90s Korean MMO field theme, downtempo tropical
    fusion, marimba and kalimba lead, warm analog pads, bongo and shaker groove,
    fretless bass, wooden flute, 98 BPM, dorian, lush but soft mix, one memorable
    melody over a steady groove, loopable short-form cue, no build-ups, steady
    tempo, no big ending, instrumental

### RO night

    torchlight camp in the jungle, late-90s Korean MMO night theme, ambient tribal
    new age, kalimba over deep pads, slow tabla-like groove, sub bass, night-insect
    textures, 72 BPM, aeolian, lush but soft mix, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

---

## city - Seabright

### MM day

    cozy harbour market town, late-90s CRPG town theme, chamber baroque, harpsichord
    and nylon guitar trading one cheerful motif, recorder lead, pizzicato strings,
    glockenspiel, upright bass walking line, light tambourine, 92 BPM, lydian, 1998
    game-soundtrack production, intimate small-room mix, loopable short-form cue,
    no build-ups, steady tempo, no big ending, instrumental

### MM night

    quiet harbour town after dusk, late-90s CRPG night town theme, warm sparse
    chamber, solo acoustic guitar lead, celesta, soft sustained strings, distant
    harpsichord, no drums, 76 BPM, major, 1998 game-soundtrack production, intimate
    mix, gentle dynamics, loopable short-form cue, no build-ups, steady tempo, no
    big ending, instrumental

### RO day

    busy welcoming town square, late-90s Korean MMO town theme, downtempo city
    groove, electric piano and nylon guitar lead, lush analog pads, soft trip-hop
    drum loop with light hats, walking upright bass, glockenspiel, 92 BPM, lydian,
    lush but soft mix, one memorable melody, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

### RO night

    lantern-lit harbour at night, late-90s Korean MMO night town theme, jazzy
    downtempo, rhodes lead, brushed kit, muted double bass, warm pads, soft
    vibraphone, 78 BPM, major seventh harmony, lush but soft mix, one melody over a
    steady groove, loopable short-form cue, no build-ups, steady tempo, no big
    ending, instrumental

---

## elf_city - Silverbough

Uses the highlands sky, so it gets day and night.

### MM day

    elven town among tall trees, late-90s CRPG town theme, refined chamber baroque,
    harp and harpsichord, flute lead on one simple motif, shimmering high strings,
    glockenspiel and triangle, no drums, 86 BPM, lydian, 1998 game-soundtrack
    production, intimate small-room mix, gentle dynamics, loopable short-form cue,
    no build-ups, steady tempo, no big ending, instrumental

### MM night

    silver moonlight over the elven town, late-90s CRPG night town theme, sparse
    chamber, harp arpeggios, celesta lead, soft high strings, warm sustained pad,
    no drums, 70 BPM, dorian, 1998 game-soundtrack production, intimate mix,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental

### RO day

    graceful elven settlement, late-90s Korean MMO town theme, downtempo new age,
    harp and glass bells over warm pads, flute lead, soft shaker and light kit,
    fretless bass, 88 BPM, lydian, lush but soft mix, one memorable melody over a
    steady groove, loopable short-form cue, no build-ups, steady tempo, no big
    ending, instrumental

### RO night

    elven town asleep, late-90s Korean MMO night theme, ambient new age, bowed pads,
    harp fragments, felt piano lead, slow soft loop, sub bass, 68 BPM, dorian, lush
    but soft mix, loopable short-form cue, no build-ups, steady tempo, no big
    ending, instrumental

---

## nomad_city - Dunehold

Uses the desert sky, so it gets day and night.

### MM day

    walled desert trade town, late-90s CRPG town theme, chamber baroque with desert
    colour, hammered dulcimer and harpsichord, ney lead on one simple motif, dry
    pizzicato strings, darbuka and tambourine, upright bass, 94 BPM, harmonic minor,
    1998 game-soundtrack production, intimate small-room mix, loopable short-form
    cue, no build-ups, steady tempo, no big ending, instrumental

### MM night

    desert town under torches, late-90s CRPG night town theme, sparse chamber, oud
    lead, low strings, single frame drum, tiny bells, 78 BPM, hijaz, 1998
    game-soundtrack production, intimate mix, gentle dynamics, loopable short-form
    cue, no build-ups, steady tempo, no big ending, instrumental

### RO day

    bustling desert bazaar, late-90s Korean MMO town theme, downtempo world groove,
    oud and qanun lead, warm analog pads, darbuka groove with claps and shaker,
    fretless bass, 96 BPM, harmonic minor, lush but soft mix, one memorable melody,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental

### RO night

    bazaar closing at night, late-90s Korean MMO night theme, ambient world fusion,
    ney over deep pads, slow handpan groove, sub bass, distant wind textures, 80 BPM,
    hijaz, lush but soft mix, one melody over a steady groove, loopable short-form
    cue, no build-ups, steady tempo, no big ending, instrumental

---

## japanese_castle - Eastern Isle Castle

### MM day

    eastern island castle grounds, late-90s CRPG zone theme, chamber baroque meeting
    eastern colour, koto ostinato instead of harpsichord, shakuhachi lead on one
    simple motif, muted strings, soft frame drum, wood block, 80 BPM, in-sen scale,
    1998 game-soundtrack production, intimate small-room mix, loopable short-form
    cue, no build-ups, steady tempo, no big ending, instrumental

### MM night

    castle courtyard by moonlight, late-90s CRPG night theme, sparse eastern chamber,
    solo koto, low shakuhachi, sustained strings, single suzu bell, no drums, 66 BPM,
    yo scale, 1998 game-soundtrack production, intimate mix, gentle dynamics,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental

### RO day

    stone castle and cherry trees, late-90s Korean MMO zone theme, downtempo eastern
    fusion, koto and shakuhachi lead, warm analog pads, soft brushed kit with wood
    percussion, fretless bass, 82 BPM, in-sen scale, lush but soft mix, one
    memorable melody over a steady groove, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

### RO night

    lanterns on the castle walls, late-90s Korean MMO night theme, ambient eastern
    new age, koto fragments over deep pads, felt piano, slow soft loop, sub bass,
    68 BPM, yo scale, lush but soft mix, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

---

## arena - The Grand Arena

### MM day

    tense crowd-lit duel, late-90s CRPG battle cue, driving chamber baroque,
    harpsichord and pizzicato string ostinato, oboe motif, tight brushed kit,
    driving upright bass, no brass fanfare, 104 BPM, aeolian, 1998 game-soundtrack
    production, dry close mix, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

### MM night

    night duel under braziers, late-90s CRPG battle cue, darker driving chamber
    baroque, low harpsichord ostinato, bass clarinet motif, tight kit, contrabass,
    single bell, no brass fanfare, 100 BPM, phrygian, 1998 game-soundtrack
    production, dry close mix, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

### RO day

    champion match in the arena, late-90s Korean MMO battle theme, downtempo battle
    groove, electric piano stabs, driving breakbeat loop, punchy bass line, synth
    lead motif, no brass, 108 BPM, aeolian, tight punchy mix, loopable short-form
    cue, no build-ups, steady tempo, no big ending, instrumental

### RO night

    night arena under braziers, late-90s Korean MMO battle theme, dark downtempo
    battle groove, gritty rhodes, breakbeat with heavy kick, sub bass, dark pads,
    104 BPM, phrygian, tight punchy mix, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

---

## church - Abandoned Church

Interior, ambient_light 0.35, no sky variants.

### MM

    abandoned church interior, late-90s CRPG holy-place theme, solemn sparse chamber,
    church organ sustained chords, low strings, tolling bell, sparse piano, distant
    wordless soprano, no percussion, 58 BPM, aeolian, 1998 game-soundtrack
    production, large stone reverb, gentle dynamics, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

### RO

    empty chapel, late-90s Korean MMO interior theme, dark ambient new age, organ
    pad, felt piano lead, deep drone, very slow soft pulse, sub bass, glass bells,
    60 BPM, aeolian, wide reverberant mix, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

---

## culverts - Seabright Culverts

Interior, ambient_light 0.3.

### MM

    damp claustrophobic stone tunnels, late-90s CRPG dungeon ambience, sparse chamber,
    low sustained strings, prepared piano drips, distant organ, soft timpani pulse,
    no melody hook, tape hiss, 62 BPM, phrygian, 1998 game-soundtrack production,
    close dry mix, loopable short-form cue, no build-ups, steady tempo, no big
    ending, instrumental

### RO

    flooded sewer tunnels, late-90s Korean MMO dungeon theme, dark ambient downtempo,
    dripping water percussion, deep drone pads, muted rhodes fragments, slow dub-like
    kick, sub bass, 64 BPM, phrygian, close dark mix, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

---

## clock_tower_1 - Clock Tower, Workshop

Interior, ambient_light 0.7, respawn_days 3.

### MM

    tinkerer workshop inside a clock tower, late-90s CRPG dungeon theme, playful
    chamber baroque, ticking clockwork percussion, music box lead on one simple
    motif, pizzicato strings, harpsichord ostinato, hammered dulcimer, 76 BPM,
    dorian, 1998 game-soundtrack production, intimate small-room mix, loopable
    short-form cue, no build-ups, steady tempo, no big ending, instrumental

### RO

    warm cluttered workshop, late-90s Korean MMO interior theme, downtempo clockwork
    groove, music box and glockenspiel lead, warm analog pads, ticking percussion
    loop, upright bass, soft brushed kit, 80 BPM, dorian, lush but soft mix, one
    memorable melody, loopable short-form cue, no build-ups, steady tempo, no big
    ending, instrumental

---

## clock_tower_2 - Clock Tower, Gearworks

Interior, ambient_light 0.6, respawn_days 3.

### MM

    grinding gearworks, late-90s CRPG dungeon theme, mechanical chamber baroque,
    offbeat clockwork percussion, harpsichord ostinato in odd meter, bass clarinet
    motif, metallic hits, low strings, 88 BPM, dorian, 1998 game-soundtrack
    production, close dry mix, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

### RO

    turning gears and steam, late-90s Korean MMO dungeon theme, downtempo industrial
    groove, metallic percussion loop, dark rhodes, warm pads under machine noise,
    driving bass line, 92 BPM, dorian, close dark mix, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

---

## clock_tower_3 - Clock Tower, Belfry Halls

Interior, ambient_light 0.6, respawn_days 3.

### MM

    high belfry among great bells, late-90s CRPG dungeon theme, airy chamber baroque,
    tubular bells and glockenspiel, flute lead on one simple motif, harp, high
    sustained strings, soft timpani, 70 BPM, aeolian, 1998 game-soundtrack
    production, tall stone reverb, gentle dynamics, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

### RO

    wind through the belfry, late-90s Korean MMO interior theme, ambient new age,
    bell tones over wide pads, felt piano lead, slow soft loop, sub bass, 68 BPM,
    aeolian, wide reverberant mix, loopable short-form cue, no build-ups, steady
    tempo, no big ending, instrumental

---

## pyramid_1 - Pyramid, Level 1

### MM

    dry sandstone corridors, late-90s CRPG dungeon ambience, sparse eastern chamber,
    low ney drone, single hammered dulcimer notes, muted low strings, soft frame
    drum, no melody hook, 64 BPM, phrygian, 1998 game-soundtrack production, close
    dry mix, loopable short-form cue, no build-ups, steady tempo, no big ending,
    instrumental

### RO

    dry stone passages, late-90s Korean MMO dungeon theme, dark ambient world, deep
    drones, sparse duduk fragments, slow frame drum pulse, sub bass, sand-like
    textures, 66 BPM, phrygian, close dark mix, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

---

## pyramid_2 - Pyramid, Level 2

### MM

    deeper trapped chambers, late-90s CRPG dungeon ambience, tense sparse chamber,
    tremolo low strings, prepared piano, distant organ, irregular soft percussion,
    no melody hook, 60 BPM, harmonic minor, 1998 game-soundtrack production, close
    dry mix, loopable short-form cue, no build-ups, steady tempo, no big ending,
    instrumental

### RO

    deeper into the tomb, late-90s Korean MMO dungeon theme, dark ambient downtempo,
    low drones, metallic scrapes, dub-like slow kick, sub bass, distant duduk,
    62 BPM, harmonic minor, close dark mix, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

---

## pyramid_3 - Pyramid, Sanctum

### MM

    sealed burial sanctum, late-90s CRPG boss-floor theme, solemn chamber with choir,
    organ pedal, wordless soprano sustains, low strings, single tolling bell, no
    percussion, 56 BPM, phrygian, 1998 game-soundtrack production, large stone
    reverb, gentle dynamics, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

### RO

    sealed sanctum, late-90s Korean MMO boss-floor theme, dark ambient ritual,
    choir-like pads, deep drone, slow heartbeat pulse, sub bass, ritual bell, 58 BPM,
    phrygian, wide dark mix, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

---

## lich_nexus - Lich Nexus

### MM

    undead nexus, late-90s CRPG endgame dungeon theme, cold dissonant chamber, organ
    clusters, harpsichord in tritones, tremolo strings, tolling bells, no percussion,
    54 BPM, octatonic, 1998 game-soundtrack production, cold wide reverb, loopable
    short-form cue, no build-ups, steady tempo, no big ending, instrumental

### RO

    necromantic nexus, late-90s Korean MMO endgame theme, dark ambient industrial,
    dissonant drone pads, reversed bell textures, very slow kick, sub bass, ghostly
    rhodes, 56 BPM, octatonic, cold wide mix, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

---

## Non-location cues

### Title / main menu - MM

    main theme of a party-based fantasy RPG, late-90s CRPG title screen, chamber
    baroque, harpsichord ostinato, oboe and recorder stating the main motif, warm
    strings, glockenspiel, acoustic guitar, 80 BPM, dorian, 1998 game-soundtrack
    production, intimate small-room mix, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

### Title / main menu - RO

    login screen of a cozy fantasy MMO, late-90s Korean MMO title theme, downtempo
    new age, nylon guitar and rhodes stating the main motif, lush analog pads, soft
    drum loop, fretless bass, 84 BPM, lydian, lush but soft mix, loopable short-form
    cue, no build-ups, steady tempo, no big ending, instrumental

### Random battle - MM

    scrappy field skirmish, late-90s CRPG battle cue, driving chamber baroque,
    harpsichord and pizzicato ostinato, oboe motif, tight brushed kit, upright bass,
    no brass fanfare, 112 BPM, aeolian, 1998 game-soundtrack production, dry close
    mix, loopable short-form cue, no build-ups, steady tempo, no big ending,
    instrumental

### Random battle - RO

    field battle, late-90s Korean MMO battle theme, downtempo battle groove,
    breakbeat loop, punchy bass, rhodes stabs, synth lead motif, 116 BPM, aeolian,
    tight punchy mix, loopable short-form cue, no build-ups, steady tempo, no big
    ending, instrumental

### Boss - MM

    a named champion stands in the way, late-90s CRPG boss cue, dark driving chamber
    baroque, low harpsichord ostinato, bass clarinet and organ motif, timpani,
    contrabass, tolling bell, no brass fanfare, 108 BPM, phrygian, 1998
    game-soundtrack production, dry close mix, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

### Boss - RO

    boss encounter, late-90s Korean MMO boss theme, dark downtempo drum and bass,
    heavy breakbeat, gritty bass, dissonant pads, sharp synth lead, 120 BPM,
    phrygian, tight punchy mix, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

### Victory sting, 8-12 s - MM

    short triumphant tag, late-90s CRPG victory jingle, chamber baroque, harpsichord
    flourish, recorder and oboe cadence, glockenspiel, one bar of timpani, 96 BPM,
    major, 1998 game-soundtrack production, intimate mix, ends clean on the downbeat,
    no fade out, instrumental

### Victory sting, 8-12 s - RO

    short victory sting, late-90s Korean MMO jingle, warm rhodes chord, glockenspiel
    run, soft kick, bright pad, 100 BPM, major, clean tight mix, ends clean on the
    downbeat, no fade out, instrumental

### Defeat sting - MM

    short somber tag, late-90s CRPG game-over cue, solo cello over organ pedal,
    single tolling bell, no percussion, 52 BPM, aeolian, 1998 game-soundtrack
    production, large reverb, ends clean, no fade out, instrumental

### Defeat sting - RO

    short defeat sting, late-90s Korean MMO game-over cue, low drone with felt piano
    fall, sub bass, 54 BPM, aeolian, wide dark mix, ends clean, no fade out,
    instrumental

### Tavern - MM

    crowded tavern, late-90s CRPG inn theme, folk chamber baroque, lute and fiddle
    lead, hand drum and tambourine, upright bass, harpsichord, foot-stomp feel,
    108 BPM, mixolydian, 1998 game-soundtrack production, intimate small-room mix,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental

### Tavern - RO

    tavern corner, late-90s Korean MMO inn theme, jazzy downtempo, rhodes and muted
    guitar lead, brushed kit, walking double bass, warm pads, soft vibraphone,
    96 BPM, major seventh harmony, lush but soft mix, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

### Merchant / shop - MM

    small shop interior, late-90s CRPG shop theme, light chamber baroque, harpsichord
    and pizzicato strings, recorder lead on one simple motif, triangle and
    glockenspiel, no drums, 88 BPM, lydian, 1998 game-soundtrack production,
    intimate small-room mix, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

### Merchant / shop - RO

    shop counter, late-90s Korean MMO shop theme, cozy downtempo, rhodes lead, soft
    kit, upright bass, warm analog pads, glockenspiel, 90 BPM, lydian, lush but soft
    mix, one memorable melody, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

---

## Boss battles

One loop per boss: the engine has no phase-switching music, so these are steady
beds with no build-ups. Five of the seven summon adds mid-fight, so the cue must
still read when the screen fills up - keep the low end clear and the motif short.

Note for the Samurai Warlord only: remove `taiko` from Exclude Styles, it is the
right drum there. Everywhere else leave the exclusion in.

### Golden Thief Bug - MM

    scuttling armoured insect thief, late-90s CRPG boss cue, nimble chamber baroque,
    fast harpsichord ostinato, skittering pizzicato strings, bassoon motif, wood
    block and shaker, upright bass, no brass fanfare, 126 BPM, dorian, 1998
    game-soundtrack production, dry close mix, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

### Golden Thief Bug - RO

    golden insect MVP fight, late-90s Korean MMO boss theme, funky downtempo
    breakbeat, slap-free funky bass line, clavinet and rhodes stabs, tight breakbeat
    loop, wood percussion, bright synth lead motif, 124 BPM, dorian, tight punchy
    mix, loopable short-form cue, no build-ups, steady tempo, no big ending,
    instrumental

### Samurai Warlord - MM

    eastern warlord duel, late-90s CRPG boss cue, driving eastern chamber ensemble,
    koto ostinato, shakuhachi motif, taiko and frame drums, low strings, wood block,
    no brass fanfare, 108 BPM, in-sen scale, 1998 game-soundtrack production, dry
    close mix, loopable short-form cue, no build-ups, steady tempo, no big ending,
    instrumental

### Samurai Warlord - RO

    warlord of the eastern isle, late-90s Korean MMO boss theme, eastern downtempo
    battle groove, koto and shamisen lead, taiko-driven breakbeat, sub bass, dark
    pads, sharp synth stabs, 112 BPM, in-sen scale, tight punchy mix, loopable
    short-form cue, no build-ups, steady tempo, no big ending, instrumental

### Gorilla Titan - MM

    towering jungle titan, late-90s CRPG boss cue, heavy chamber baroque, low
    marimba and contrabass ostinato, bass clarinet motif, tribal frame drums and
    log drums, tremolo low strings, no brass fanfare, 100 BPM, dorian, 1998
    game-soundtrack production, dry close mix, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

### Gorilla Titan - RO

    titan of the deep jungle, late-90s Korean MMO boss theme, tribal downtempo
    battle groove, heavy log drum and bongo groove, sub bass, dark warm pads, low
    kalimba motif, distorted-free synth lead, 104 BPM, dorian, tight punchy mix,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental

### Orc Warlord - MM

    orc warlord and his retainers, late-90s CRPG boss cue, martial chamber baroque,
    harpsichord and low string ostinato, oboe war motif, timpani and hand drums,
    contrabass, single war horn-free bell, 116 BPM, aeolian, 1998 game-soundtrack
    production, dry close mix, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

### Orc Warlord - RO

    orc hero MVP fight, late-90s Korean MMO boss theme, driving downtempo drum and
    bass, heavy breakbeat, gritty bass line, dark pads, aggressive synth lead motif,
    war drum layer, 122 BPM, aeolian, tight punchy mix, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

### Brood Mother - MM

    dragon brood mother on the cliffs, late-90s CRPG boss cue, dark heavy chamber
    baroque, low harpsichord and organ ostinato, contrabassoon motif, tremolo
    strings, timpani, tolling low bell, no brass fanfare, 104 BPM, phrygian, 1998
    game-soundtrack production, large stone reverb, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

### Brood Mother - RO

    brood mother of the dragon cliffs, late-90s Korean MMO boss theme, dark
    downtempo drum and bass, heavy breakbeat with tribal layer, sub bass, dissonant
    dark pads, duduk motif, sharp synth stabs, 118 BPM, phrygian, tight punchy mix,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental

### Ancient God of Death - MM

    ancient god of death awakens, late-90s CRPG boss cue, solemn dissonant chamber
    with choir, organ clusters, wordless soprano and low male choir, tremolo
    strings, tolling bells, slow timpani, no brass fanfare, 96 BPM, octatonic, 1998
    game-soundtrack production, vast stone reverb, loopable short-form cue, no
    build-ups, steady tempo, no big ending, instrumental

### Ancient God of Death - RO

    death god ritual fight, late-90s Korean MMO boss theme, dark ritual downtempo,
    choir-like pads, deep drone, slow heavy breakbeat, sub bass, reversed bell
    textures, dissonant rhodes motif, 100 BPM, octatonic, wide dark mix, loopable
    short-form cue, no build-ups, steady tempo, no big ending, instrumental

### Alien Enforcer - MM

    something not of this world under the sea, late-90s CRPG boss cue, chamber
    baroque invaded by cold electronics, harpsichord ostinato against analog synth
    drones, detuned bell motif, tremolo strings, metallic percussion, no brass
    fanfare, 112 BPM, whole-tone, 1998 game-soundtrack production, cold wide reverb,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental

### Alien Enforcer - RO

    alien enforcer in the deep, late-90s Korean MMO boss theme, dark electronic
    drum and bass, analog synth bass line, cold detuned pads, sharp digital lead
    motif, glitchy metallic percussion, no acoustic instruments, 126 BPM,
    whole-tone, tight punchy mix, loopable short-form cue, no build-ups, steady
    tempo, no big ending, instrumental

## Arena champion duels

Four champions ride the same arena, each across the difficulty tiers. The arena
day/night cues cover the room; these are for the duel itself. One base cue per
house plus a one-line swap per champion.

### Champion duel - MM

    one-on-one champion duel, late-90s CRPG duel cue, lean driving chamber baroque,
    harpsichord and pizzicato ostinato, oboe motif, tight brushed kit, driving
    upright bass, no brass fanfare, 110 BPM, aeolian, 1998 game-soundtrack
    production, dry close mix, loopable short-form cue, no build-ups, steady tempo,
    no big ending, instrumental

### Champion duel - RO

    one-on-one champion duel, late-90s Korean MMO duel theme, driving downtempo
    breakbeat, rhodes stabs, punchy bass line, dark warm pads, synth lead motif,
    112 BPM, aeolian, tight punchy mix, loopable short-form cue, no build-ups,
    steady tempo, no big ending, instrumental

Per-champion swaps - replace the lead instrument and the mode in either cue:

- **hobbit_archer** - lead `tin whistle` (MM) / `plucked synth` (RO), mode
  `mixolydian`, add `light shaker`, 116 BPM. Quick and light.
- **dark_elf_sorceress** - lead `celesta and low organ` (MM) / `cold detuned pad
  lead` (RO), mode `harmonic minor`, add `tolling bell`, 104 BPM. Cold and arcane.
- **wild_druid** - lead `low flute and hand drums` (MM) / `kalimba and frame drum`
  (RO), mode `dorian`, add `log drum layer`, 108 BPM. Earthy and feral.
- **weapon_master** - lead `oboe and pizzicato strings` (MM) / `gritty rhodes` (RO),
  mode `phrygian`, add `tight snare rolls`, 118 BPM. Precise and relentless.

## Retro variant (MM3-5, Xeen era)

Any prompt above becomes the Xeen-era sound by swapping the whole instrument list
for the line below and keeping the mood, BPM and mode:

    FM synthesis AdLib OPL2 chiptune, square and sawtooth leads, 4-op FM bells,
    no acoustic instruments, 1992 DOS game soundtrack

Example, forest day:

    pastoral idyllic woodland exploration, 1992 DOS CRPG field theme, FM synthesis
    AdLib OPL2 chiptune, square and sawtooth leads, 4-op FM bells, no acoustic
    instruments, one simple singable motif, 84 BPM, dorian, dry mono-ish mix,
    loopable short-form cue, no build-ups, steady tempo, no big ending, instrumental
