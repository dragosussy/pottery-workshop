# Glaze notebook: design

What the app does and how it's built. Getting started is in the
[README](README.md).

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
  There's one kiln, so at most one load is in progress: it's being loaded, then
  firing, then unloaded.
- **Position:** where a piece sat in that load. The kiln is about 5 units wide,
  3 deep and 3 high, so a position is a height (bottom, middle or top) and a
  point on a map of the shelf at that height, tapped by eye. It's recorded per
  piece, because that's where the kiln differs. Edge or middle is worked out
  from the point.

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
- **Records made after firing are marked.** A piece logged once it was already
  fired shows "Logged after firing", so a detail remembered later never looks
  like one written down at the time.

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

**Loading the kiln.** The Kiln tab shows the one kiln in whichever state it's
in. "Start loading" brings up a map of its three heights, seen from above, bottom
shelf first as that's the order it's loaded.
Choose a piece from those to be glazed and tap where it sits, at whichever
height, and the next piece is chosen for you. "Start firing" then
asks for the schedule. This is also the last reminder: anything going in that isn't on the list gets logged
now, while it's still in front of you.

**Unloading the kiln.** "Unload" starts a walk through its pieces shelf by
shelf, showing where each sat, with a "next" button: take a photo, tap a rating, tick any defects.
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

**Choosing a glaze.** A material's tile folds open to its results as a photo grid,
grouped by clay body, so both what it does and what's never been tried are
visible. Each photo is captioned with its schedule and position, which can
also be filtered on, so a result that only happens on the hot shelf is easy to
spot. A keepers gallery shows everything rated great.

**Mixing a batch.** A recipe is written in parts by weight, split into the base
and what goes on top of it, and shows each as a share of the base. Say how much
base to mix and it gives the weight of each ingredient and the water to start
with.

**Clearing the backlog.** Answering "Already fired" on the log form asks for a
rating and leaves unknown details blank. It remembers the last clay body and
offers "Save and add another".

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
  water REAL,                      -- to start with, in the same parts as the lines
  consistency TEXT NOT NULL DEFAULT '',  -- what to aim for once mixed
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
  state TEXT NOT NULL,             -- loading | firing | unloaded; one unfinished at a time
  fired_on TEXT,                   -- YYYY-MM-DD, set when firing starts if not given
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
  pos_x REAL,                      -- 0–1, from the left of the shelf
  pos_y REAL,                      -- 0–1, from the front of the kiln
  spot TEXT,                       -- edge | middle, worked out from pos_x and pos_y
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,  -- never reused: photos are cached by id
  piece_id INTEGER NOT NULL REFERENCES pieces(id) ON DELETE CASCADE,
  stage TEXT NOT NULL,             -- before | after
  created_at INTEGER NOT NULL
);
```

- **A piece's state is derived:** a rating means `done`. Otherwise no firing
  means `bench` (shown as "To be glazed"), a firing still in progress means
  `kiln`, and an unloaded firing means `waiting` for its result. An old tile
  always gets a rating, since it's already fired.
- **Only a load that's being loaded takes new pieces.**
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
- **Water** belongs to a recipe version: an amount to start with, in the same
  parts as the ingredients, and the consistency to aim for, since the rest is
  done by feel. It isn't recorded per layer.
- **Oxide washes** are recipes too: the oxide, anything else in it such as CMC
  or a thickener, and its water, scaled from how much water you use.
- **Unknown materials:** "Unknown blue" is a legitimate material for the
  backlog. That keeps every layer linked to something searchable.
- **Photos** are files `photos/<id>.jpg`, processed on the server as in
  humidity, plus `photos/<id>.thumb.jpg` at about 400 px. Humidity dropped
  thumbnails on purpose, but a grid of a hundred full-size photos over a phone
  connection isn't usable.

## 6. Screens

Vue on a phone-first layout, like humidity. Four tabs sit at the bottom of
every screen except forms and the result walk, and Back always returns to where
you came from.

| Tab | What's on it |
|---|---|
| Now | Find a tile by number, "Log a piece", results still to enter, the kiln's status, what's to be glazed |
| Kiln | The one kiln: empty, being loaded (the map and what's to place) or firing. Past firings are a link |
| Pieces | Search, quick filters for keepers, tiles and pots, and more filters folded away |
| Glazes | Materials by kind, each a tile that folds open in place to its results per clay body, recipe, versions and batch scaling |

A piece shows its journey (To be glazed, In the kiln, Fired) and one main
action for where it is on it: add what went on, enter the result, or do this
again.

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
hand-numbered tile during a session and finding it again from the object. A
spreadsheet handles recipes but is poor at photos, ordered stacks and fast
phone entry.

## 8. How it's built, and what it touches

Same stack as humidity: one Go binary embedding a Vue app, SQLite through
modernc, in `apps/glaze/`, and the rules in
[Architecture](../../architecture.md#rules-every-app-follows) apply, including
using the [shared code](../../architecture.md#shared-code).
`GET /api/summary` returns the counts for the homepage tile: to be glazed, in
the kiln and waiting for a result. There's no hardware code, so no simulator either.

| Area | Implication |
|---|---|
| **Data safety** | Backups today cover only the database and stay on the SD card. Here, photos are the record, so **glaze depends on the [off-device backups](../../architecture.md#data), which include photos,** being set up and restored successfully at least once before real data goes in. |
| **Availability** | The Pi stays on from now on, so the notebook is there whenever people glaze or want to look something up. |
| **Hardware** | The app doesn't touch the pump, sensors or screen. But with the Pi always on, humidity's pump control runs whenever a target is saved, so a target left set on an empty shelf keeps misting. |
| **Other apps** | Glaze is an ordinary route at `/glaze/`, with its data in `data/glaze/`. It doesn't talk to humidity. |
| **Use** | Two or three people, mostly on their own phones. Recently used materials are worked out from the layers, so everyone sees the same list. If two people save the same record, the last save wins, which is fine at this scale. |
| **Storage** | Around 0.5 MB per photo, so thousands of photos fit, but on the SD card. |

Tests cover what would quietly damage the records: frozen recipe versions,
duplicate numbers (and re-fires keeping theirs), state derivation, archive
instead of delete and batch scaling, plus one HTTP walk-through from logging
to result.

## 9. HTTP API

JSON in and out, with errors as `{"error": "message"}`. A piece is saved whole:
the client sends the full record, layers included.

| Method + path | What it does |
|---|---|
| `GET /api/summary` | The homepage tile |
| `GET /api/suggestions` | Ingredients, schedules and cones typed before |
| `GET, POST /api/clay-bodies`, `PUT, DELETE …/{id}` | Clay bodies; deleting one in use is a 409 |
| `GET, POST /api/materials`, `GET, PUT, DELETE …/{id}` | Materials; deleting one in use is a 409 |
| `PUT /api/materials/{id}/recipe` | Corrects the newest version if unused, otherwise adds the next |
| `GET, POST /api/firings`, `GET, PUT, DELETE …/{id}` | Firings; a second load in progress or deleting one with pieces is a 409 |
| `PUT /api/firings/{id}/state` | Moves a load between loading, firing and unloaded |
| `GET /api/pieces` | Filters: `number`, `state`, `kind`, `material`, `clay`, `firing`, `schedule`, `level`, `spot`, `rating`, `defect`, `q` |
| `POST /api/pieces`, `GET, PUT, DELETE …/{id}` | One piece; a tile number already taken is a 409 |
| `POST /api/pieces/{id}/photos?stage=` | Adds a photo: the raw image, up to 25 MB |
| `GET /api/photos/{id}`, `…/thumb`, `DELETE …/{id}` | A photo and its thumbnail |
| `GET /api/backup` | A fresh copy of the database |

Lists come newest first, 60 at a time with `offset`. `?firing=` returns the
whole load instead, bottom shelf first.

The server's configuration is `ADDR` (default `:8080`) and `DATA_DIR` (default
`/data`).
