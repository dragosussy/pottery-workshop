# Copies every app's data to Cloudflare R2 once a day, starting when the
# container starts, so a restart can't skip a day. Build it from the repo root,
# like the apps.

FROM rclone/rclone:1.73.1
COPY backup/backup.sh /backup.sh
ENTRYPOINT ["sh", "-c", "while :; do nice -n 19 /backup.sh; sleep 86400; done"]
