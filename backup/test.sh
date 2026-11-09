#!/bin/sh
# Tests for backup.sh. Runs inside the backup image (see the root Makefile),
# with a local folder standing in for the R2 bucket and a stand-in wget that
# records the success report.

set -eu

bucket=/tmp/bucket
pings=/tmp/pings
today=$(date -u +%F)

export RCLONE_CONFIG_R2_TYPE=local
export BACKUP_BUCKET=$bucket
export BACKUP_PING_URL=http://ping.test/check

mkdir -p /tmp/bin
cat >/tmp/bin/wget <<EOF
#!/bin/sh
echo ping >>$pings
EOF
chmod +x /tmp/bin/wget
export PATH=/tmp/bin:$PATH

fail() {
	echo "FAIL: $1"
	exit 1
}

reset() {
	rm -rf /data/* "$bucket"
	: >"$pings"
	mkdir -p /data/humidity/photos /data/humidity/backups
	echo database >/data/humidity/humidity.db
	echo wal >/data/humidity/humidity.db-wal
	echo shm >/data/humidity/humidity.db-shm
	echo photo-one >/data/humidity/photos/1.jpg
	echo first-of-month >/data/humidity/backups/2026-11-01.db
	echo today >/data/humidity/backups/2026-11-09.db
}

expect_success() {
	/backup.sh >/dev/null 2>&1 || fail "$1: backup.sh failed"
	[ "$(wc -l <"$pings")" -eq 1 ] || fail "$1: expected one success report"
	: >"$pings"
}

expect_failure() {
	if /backup.sh >/dev/null 2>&1; then fail "$1: backup.sh succeeded"; fi
	[ ! -s "$pings" ] || fail "$1: reported success after a failure"
}

exists() { [ -f "$bucket/$1" ] || fail "missing from the backup: $1"; }

# The right files go to the right places, and live databases don't go at all.
reset
expect_success "normal run"
exists snapshots/humidity/2026-11-01.db
exists snapshots/humidity/2026-11-09.db
exists monthly/humidity/2026-11-01.db
exists files/humidity/photos/1.jpg
[ ! -e "$bucket/monthly/humidity/2026-11-09.db" ] || fail "a non-monthly snapshot went to monthly/"
[ ! -e "$bucket/files/humidity/backups" ] || fail "snapshots were copied into files/"
[ -z "$(find "$bucket" -name 'humidity.db*')" ] || fail "a live database was uploaded"

# A replaced photo keeps its old version.
echo photo-one-new >/data/humidity/photos/1.jpg
expect_success "replaced photo"
[ "$(cat "$bucket/files/humidity/photos/1.jpg")" = photo-one-new ] || fail "the new photo wasn't uploaded"
[ "$(cat "$bucket/replaced/$today/humidity/photos/1.jpg")" = photo-one ] || fail "the old photo wasn't kept"

# Deleting something on the Pi doesn't delete it from the backup.
rm /data/humidity/photos/1.jpg
expect_success "deleted photo"
exists files/humidity/photos/1.jpg

# An app that stopped writing snapshots is a failure, and the others still
# get backed up.
reset
mkdir -p /data/glaze/backups
echo old >/data/glaze/backups/2020-01-01.db
touch -d "2020-01-01 00:00:00" /data/glaze/backups/2020-01-01.db
expect_failure "stale snapshot"
exists snapshots/humidity/2026-11-09.db

# So is an app with no snapshots at all.
reset
mkdir -p /data/glaze
expect_failure "no snapshots folder"

# No apps at all means the data folder is wrong.
reset
rm -rf /data/*
expect_failure "empty data folder"

# So does a failed upload.
reset
RCLONE_CONFIG_R2_TYPE=nosuchbackend expect_failure "upload error"

echo "backup: all tests passed"
