#!/bin/sh
# Copies every app's data off the Pi, once, then reports success. Any failure
# means no report, and the missing report is what raises the alarm.
#
# It always uses "copy", never "sync", so something deleted on the Pi is never
# deleted from the backup. Live databases are never uploaded, because a copy
# taken while SQLite is writing can be corrupt. Only the snapshots each app
# writes to backups/ go up.

set -u

remote="r2:$BACKUP_BUCKET"
today=$(date -u +%F)
# Gentle on the Pi and on the studio's connection.
flags="--bwlimit 2M --transfers 2 --checkers 2"

backup_app() {
	app=$1
	dir=/data/$app

	# With no new snapshot there's nothing to copy and the copy would succeed,
	# hiding an app that has stopped writing them.
	if [ -z "$(find "$dir/backups" -name '*.db' -mtime -2 2>/dev/null)" ]; then
		echo "$app: no snapshot from the last 2 days in $dir/backups"
		return 1
	fi

	# Bucket rules expire snapshots/ after 90 days and keep monthly/ forever.
	# A snapshot stays on the Pi for a week, so a run that misses the 1st
	# still picks that day's up.
	rclone copy "$dir/backups" "$remote/snapshots/$app" --include '*.db' $flags &&
		rclone copy "$dir/backups" "$remote/monthly/$app" --include '*-01.db' $flags &&
		rclone copy "$dir" "$remote/files/$app" $flags \
			--exclude '/backups/**' --exclude '/*.db' --exclude '/*.db-wal' --exclude '/*.db-shm' \
			--backup-dir "$remote/replaced/$today/$app"
}

apps=0
failed=0
for path in /data/*/; do
	[ -d "$path" ] || continue
	app=$(basename "$path")
	apps=$((apps + 1))
	if backup_app "$app"; then
		echo "$app: backed up"
	else
		echo "$app: FAILED"
		failed=1
	fi
done

# No apps at all means the data folder isn't where it should be.
if [ "$apps" -eq 0 ]; then
	echo "no apps found in /data"
	exit 1
fi
if [ "$failed" -ne 0 ]; then
	exit 1
fi

wget -q -O /dev/null "$BACKUP_PING_URL" || {
	echo "backed up, but the success report didn't get through"
	exit 1
}
