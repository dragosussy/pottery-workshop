# Glaze notebook: design

**Status: draft, nothing is built yet.** This page sets out what the app is
for and how it would work.

## 1. The problem

The studio keeps getting surfaces it can't reproduce. A piece comes out of the
kiln looking great, and by then nobody remembers exactly what went on it.

What makes this hard here:

- **A surface is a stack, not a glaze.** A piece can have a slip, then an oxide,
  then a commercial brush glaze, then a glaze we mixed ourselves. The order,
  how much of each and where it went all change the result.
- **The clay matters.** The same stack can behave differently on each of our
  3 or 4 clay bodies, so "this glaze is nice" is only true for one clay body.
- **The firing matters, and so does the spot.** Schedules vary. Our kiln is
  self-built and rectangular, usually loaded with two shelves and at most
  three. Its temperature isn't even, from the edges to the middle or from the
  top shelf to the bottom, so two pieces in one load can have fired quite
  differently.
- **The information is spread over weeks.** Slip often goes on at leather hard.
  Glaze goes on after the bisque. The result is only known days later, and
  that is the moment people want the information.
- **Our own recipes change.** After an adjustment, an old test tile no longer
  describes what's in the recipe book.
- **There's a backlog.** Many hand-numbered test tiles exist with no record in
  any system.

## 2. What the app has to do

In order of importance:

1. **Make it quick to record what was done, at the moment it's done.** About
   30 seconds on a phone, with messy hands. If logging is slow, people won't
   do it and nothing else here matters.
2. **Answer "what did we do to get this?"** from the object in your hand,
   as the steps to repeat it.
3. **Answer "what do we know about this glaze?"** with results side by side,
   per clay body.
4. **Keep recipes and their adjustments**, and always know which version a
   test used.
5. **Accept incomplete records**, because the backlog and forgotten pieces
   will have gaps. A partial record is still worth having.

## 3. The approach

One idea carries the design: **anything that goes in the kiln gets one record,
and that record is the stack, the clay, the firing and the result together.**
A test tile and a finished pot are the same kind of record. So a pot that
comes out great is as well documented as a planned test, and it can be
compared with the tiles.

```
clay body ──┐
            ├── piece (tile or pot) ──< layers >── material ──< recipe versions
firing ─────┘        │                                              (optional)
                     ├── result: rating, defects, notes
                     └── photos: before and after firing
```

- **Piece:** one object going in the kiln. A test tile is identified by the
  number written on it, and a pot by its photo.
- **Layer:** one thing applied to it, in order: which material, how it was
  applied, how thick and where.
- **Material:** anything we put on clay: oxide, slip, commercial glaze or our
  own glaze. This is one list, so a layer picker is one list too.
- **Recipe version:** the ingredients of a material we mix ourselves.
  Commercial products have none.
- **Firing:** one kiln load, recorded once and shared by everything in it.
- **Position:** where a piece sat in that load: bottom, middle or top shelf,
  and at the edge or in the middle of it. It's recorded per piece, because
  that's where the kiln differs.

### Rules that keep the records trustworthy

- **Recipes never change under an old result.** A version that any layer uses
  is frozen. Saving a change creates the next version, with a note saying why.
- **Materials in use are archived, not deleted.** They leave the pickers but
  stay on the old records.
- **The app never assigns a tile number.** People write numbers on tiles by
  hand, and it should only record what's on the tile. It can suggest the
  highest number + 1, but that can be wrong until the backlog is entered, so
  a duplicate is refused with a link to the existing tile.
- **Only the identity of a piece is required:** a number for a tile, a photo
  for a pot. Clay, layers, firing, position and result can all be filled in
  later or never.
- **Records from memory are marked.** One flag says a piece was filled in
  afterwards, so a guess never looks like a measurement.

### Glaze thickness

We judge thickness by feel and don't measure specific gravity, so it's
recorded the way humidity records dryness: a continuous value from 0.0 to 5.0,
to one decimal, on the same slider, read out in words. The whole numbers are
named landmarks:

| value | label |
|---|---|
| 0 | Trace: the clay clearly shows through |
| 1 | Very thin |
| 2 | Thin |
| 3 | Medium |
| 4 | Thick |
| 5 | Very thick: at risk of running or crawling |

It's a judgement per layer, not a calculation, and it applies to every kind of
material. Brushed and sponged layers can also record the number of coats,
since that's a plain fact worth repeating.

## 4. How it gets used

**Glazing a piece.** Open the app, tap "Log a piece", type the tile number or
take a photo of the pot, and pick the clay body. Add layers from a list with
recently used materials first. Method and thickness start at what that material
last had, so a typical layer is one or two taps. A material that isn't in the
list can be added by name from the same screen. Slip put on at leather hard
starts a record, and the glaze layers join it weeks later.

**Loading the kiln.** Create the firing, choosing its schedule from earlier
ones or typing a new one. Then, shelf by shelf, tap the pieces going on that
shelf from the list of those on the bench, and mark edge or middle. This is
also the last reminder: anything going in that isn't on the list gets logged
now, while it's still in front of you.

**Unloading the kiln.** The firing page walks through its pieces shelf by
shelf, with a "next" button: take a photo, tap a rating, tick any defects.
Pots are identified by the photo taken when they were logged, together with
where they sat. A photo alone can't separate similar pots with different
stacks, since raw glaze looks pale and chalky, so position does real work here.

**Holding a mystery tile.** The home screen opens on a number box. Type 142
and you get the tile: photo, clay, the stack in order, the firing, its
position and the recipe versions used.

**Holding a mystery pot.** Pots have no number, so they're found by eye:
browse pots by firing or newest first, and narrow by clay body or rating.

**Doing it again.** Every piece has "Do this again", which starts a new record
with the same clay and stack filled in, and shows the schedule and position
to aim for. Changing one thing from there is also how a variation gets tested.

**Re-firing.** It's uncommon but possible, so it costs almost nothing in the
design. "Re-fire" starts a new record linked to the old one, with the stack
copied and new layers added on top. The new record gets its own firing,
position, photos and result, and the old one stays as it came out. A tile
keeps its number, and looking it up shows the latest record with a link back.

**Choosing a glaze.** A material's page shows its results as a photo grid,
grouped by clay body, so both what it does and what's never been tried are
visible. Each photo is captioned with its schedule and position, which can
also be filtered on, so a result that only happens on the hot shelf is easy to
spot. A keepers gallery shows everything rated great.

**Mixing a batch.** A recipe page scales to a batch size: enter 3000 g and get
the weight of each ingredient.

**Clearing the backlog.** A short form: number, clay, photo, rating, and
whatever's known about the stack. It remembers the last clay body and offers
"Add another". Records are marked as from memory by default.

## 5. Data

SQLite at `$DATA_DIR/glaze.db`, set up and versioned the same way as humidity.
Times are unix seconds.

```sql
CREATE TABLE clay_bodies (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  notes TEXT NOT NULL DEFAULT '',
  archived_at INTEGER
);
CREATE TABLE materials (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,              -- oxide | slip | commercial | own
  maker TEXT NOT NULL DEFAULT '',  -- commercial: brand and colour code
  notes TEXT NOT NULL DEFAULT '',
  archived_at INTEGER
);
CREATE TABLE recipes (             -- one row per version
  id INTEGER PRIMARY KEY,
  material_id INTEGER NOT NULL REFERENCES materials(id),
  version INTEGER NOT NULL,
  change_note TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE (material_id, version)
);
CREATE TABLE recipe_lines (
  id INTEGER PRIMARY KEY,
  recipe_id INTEGER NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  ingredient TEXT NOT NULL,        -- free text, suggested from earlier lines
  amount REAL NOT NULL,            -- parts by weight
  addition INTEGER NOT NULL DEFAULT 0  -- 1 = on top of the 100 % base
);
CREATE TABLE firings (
  id INTEGER PRIMARY KEY,
  fired_on TEXT,                   -- YYYY-MM-DD, null if not known
  cone TEXT NOT NULL DEFAULT '',
  schedule TEXT NOT NULL DEFAULT '',  -- free text, suggested from earlier firings
  notes TEXT NOT NULL DEFAULT ''
);
CREATE TABLE pieces (
  id INTEGER PRIMARY KEY,
  kind TEXT NOT NULL,              -- tile | pot
  number INTEGER,                  -- as written on the tile; null for pots
  clay_body_id INTEGER REFERENCES clay_bodies(id),
  firing_id INTEGER REFERENCES firings(id),
  level TEXT,                      -- bottom | middle | top
  spot TEXT,                       -- edge | middle
  refire_of INTEGER REFERENCES pieces(id),
  notes TEXT NOT NULL DEFAULT '',
  from_memory INTEGER NOT NULL DEFAULT 0,
  rating TEXT,                     -- great | fine | failed
  result_notes TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX pieces_number ON pieces(number) WHERE refire_of IS NULL;
CREATE TABLE layers (
  id INTEGER PRIMARY KEY,
  piece_id INTEGER NOT NULL REFERENCES pieces(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,       -- 1 is closest to the clay
  material_id INTEGER NOT NULL REFERENCES materials(id),
  recipe_id INTEGER REFERENCES recipes(id),  -- the version used, if it has one
  stage TEXT,                      -- greenware | bisque | refire
  method TEXT,                     -- brush | dip | pour | spray | sponge
  thickness REAL,                  -- 0.0–5.0, one decimal
  coats INTEGER,                   -- brush and sponge only
  area TEXT NOT NULL DEFAULT '',   -- e.g. "inside", "rim", "left half"
  notes TEXT NOT NULL DEFAULT ''
);
CREATE TABLE piece_defects (
  piece_id INTEGER NOT NULL REFERENCES pieces(id) ON DELETE CASCADE,
  defect TEXT NOT NULL,            -- crawling, pinholes, running, crazing, ...
  PRIMARY KEY (piece_id, defect)
);
CREATE TABLE photos (
  id INTEGER PRIMARY KEY,
  piece_id INTEGER NOT NULL REFERENCES pieces(id) ON DELETE CASCADE,
  stage TEXT NOT NULL,             -- before | after
  created_at INTEGER NOT NULL
);
```

- **A piece's state is derived:** a rating means `done`, otherwise a firing
  means `waiting for result`, otherwise `on the bench`. The backlog form
  always asks for a rating, since those tiles are already fired.
- **A tile needs a number and a pot needs a photo.** The server checks the
  number, and the log form won't save a pot without a photo.
- **A re-fire** copies the earlier record's layers, keeps its tile number and
  sets `refire_of`. Layers added for it have the stage `refire`.
- **Shelves are stored by level, not by number.** The second shelf is the top
  of a two-shelf load but the middle of a three-shelf one, so a number wouldn't
  compare across firings.
- **A layer's version** starts as the newest one, and only shows a choice when
  the material has more than one.
- **Scaling a recipe** treats non-additions as the base. Each amount becomes
  `amount / base total × batch size`, for additions as well.
- **Oxide washes** are recipes too, with water as an ingredient.
- **Unknown materials:** "Unknown blue" is a legitimate material for the
  backlog. That keeps every layer linked to something searchable.
- **Photos** are files `photos/<id>.jpg`, processed on the server as in
  humidity, plus `photos/<id>.thumb.jpg` at about 400 px. Humidity dropped
  thumbnails on purpose, but a grid of a hundred full-size photos over a phone
  connection isn't usable.

## 6. Screens

Vue on a phone-first layout, like humidity.

| Screen | What's on it |
|---|---|
| Home | Find-by-number box, "Log a piece", on the bench, waiting for result by firing |
| Piece | Photos, the stack in order as steps to repeat, clay, firing and position, result, "Do this again", "Re-fire" |
| Log / edit | The capture form from §4; one short variant for the backlog |
| Firing | Details, its pieces by shelf, loading by shelf, the one-by-one result walk |
| Browse | Thumbnail grid of tiles or pots, filtered by material, clay body, schedule, shelf level, edge or middle, rating, defect or text; keepers is one filter |
| Materials | The list, by kind |
| Material | Recipe with versions and batch scaling, results grouped by clay body |
| Settings | Clay bodies, backup download |

## 7. Left out on purpose

- **Glaze chemistry** (unity formulas, oxide analysis, substitutions), raw
  material stock and costs. None of them help remember what happened.
- **Specific gravity and tracking buckets.** We go by feel, and thickness
  covers that. One catch: after a recipe changes, a layer starts on the newest
  version even if an older mix is still in the bucket, so someone has to pick
  the older version.
- **Test series** as a concept (a line blend, say). "Do this again" plus
  consecutive tile numbers and notes covers this for now.
- **Bulk import.** The backlog will be typed in.
- **A link to the humidity app's pieces.** Those end at trimming, this starts
  at glazing, and connecting them adds coupling for little gain.
- **Logins, a record of who logged what, offline mode and food-safety
  tracking.** A leach test result can go in the notes, but the app makes no
  claim about whether a surface is safe.

**Why not an existing tool or a spreadsheet?** Recipe sites such as Glazy are
built around recipes and chemistry. Our gap is capturing a stack against a
hand-numbered tile during a session and finding it again from the object. I
haven't evaluated Glazy in depth, so it's worth a look before building. A
spreadsheet handles recipes but is poor at photos, ordered stacks and fast
phone entry.

## 8. How it's built, and what it touches

Same stack as humidity: one Go binary embedding a Vue app, SQLite through
modernc, in `apps/glaze/`, and the rules in
[Architecture](../../architecture.md#rules-every-app-follows) apply, including
using the [shared code](../../architecture.md#shared-code).
`GET /api/summary` returns the counts for the homepage tile: on the bench and
waiting for result. No new dependencies are expected, and there's no hardware
code, so no simulator either.

| Area | Implication |
|---|---|
| **Data safety** | Backups today cover only the database and stay on the SD card. Here, photos are the record, so **glaze depends on the [off-device backups](../../architecture.md#data), which include photos,** being set up and restored successfully at least once before real data goes in. |
| **Availability** | The Pi stays on from now on, so the notebook is there whenever people glaze or want to look something up. |
| **Hardware** | The app doesn't touch the pump, sensors or screen. But with the Pi always on, humidity's pump control runs whenever a target is saved, so a target left set on an empty shelf keeps misting. |
| **Other apps** | The gateway goes in first. It moves humidity to `/humidity/` and its data to `data/humidity/`, which is a manual step on the Pi. Glaze is then an ordinary route at `/glaze/`, with its data in `data/glaze/`. |
| **Use** | Two or three people, mostly on their own phones. Recently used materials are worked out from the layers, so everyone sees the same list. If two people save the same record, the last save wins, which is fine at this scale. |
| **Repo changes** | `deploy.sh` takes an app name, the root `Makefile` gets the new app, and `go.mod` gets another `ignore` line for its `node_modules`. |
| **Storage** | Around 0.5 MB per photo, so thousands of photos fit, but on the SD card. |
| **Docs** | The README table, architecture status table and deployment page change when the app ships, and so does humidity's design page, which says the Pi is switched off between sessions. This page becomes the description of what was built. |

Tests cover what would quietly damage the records: frozen recipe versions,
duplicate numbers (and re-fires keeping theirs), state derivation, archive
instead of delete and batch scaling, plus one HTTP walk-through from logging
to result.

## 9. Build order

The gateway comes first, as its own change. Each step after it is usable by
itself.

1. **Capture and lookup:** clay bodies, materials, pieces, layers with
   thickness, photos, find by number, the backlog form. Off-device backups
   have to work by the end of this step, because real data starts here.
2. **The kiln cycle:** firings, loading by shelf, the result walk, the bench
   and waiting lists, "Do this again", "Re-fire".
3. **Recipes:** versions, the freeze rule, batch scaling, choosing a version
   on a layer.
4. **Finding things:** browse and filters, keepers, a material's results by
   clay body.
