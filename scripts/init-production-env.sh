#!/bin/sh
set -eu

source_env=${1:-/opt/lighttech_deploy/.env}
destination=${2:-.env.production}

if [ -s "$destination" ]; then
  echo "$destination already exists; leaving production secrets unchanged"
  exit 0
fi
if [ ! -r "$source_env" ]; then
  echo "COS environment file is not readable: $source_env" >&2
  exit 1
fi

read_value() {
  awk -F= -v requested="$1" '$1 == requested { print substr($0, index($0, "=") + 1); exit }' "$source_env"
}

cos_secret_id=$(read_value COS_SECRET_ID)
cos_secret_key=$(read_value COS_SECRET_KEY)
cos_region=$(read_value COS_REGION)
cos_bucket=$(read_value COS_BUCKET)

if [ -z "$cos_secret_id" ] || [ -z "$cos_secret_key" ] || [ -z "$cos_region" ] || [ -z "$cos_bucket" ]; then
  echo "COS_SECRET_ID, COS_SECRET_KEY, COS_REGION and COS_BUCKET are required" >&2
  exit 1
fi

random_hex() {
  openssl rand -hex "$1"
}

postgres_password=$(random_hex 32)
publish_token=$(random_hex 48)
temporary="${destination}.tmp.$$"
umask 077
{
  printf 'LEARNING_POSTGRES_PASSWORD=%s\n' "$postgres_password"
  printf 'LEARNING_CMS_PUBLISH_TOKEN=%s\n' "$publish_token"
  printf 'LEARNING_APPLE_CLIENT_ID=com.oldj.englishlearning\n'
  printf 'LEARNING_OBJECT_ENDPOINT=cos.%s.myqcloud.com\n' "$cos_region"
  printf 'LEARNING_OBJECT_ACCESS_KEY=%s\n' "$cos_secret_id"
  printf 'LEARNING_OBJECT_SECRET_KEY=%s\n' "$cos_secret_key"
  printf 'LEARNING_OBJECT_BUCKET=%s\n' "$cos_bucket"
  printf 'LEARNING_PUBLISHED_OBJECT_BUCKET=%s\n' "$cos_bucket"
  printf 'LEARNING_USER_MEDIA_BUCKET=%s\n' "$cos_bucket"
  printf 'LEARNING_BACKUP_INTERVAL_SECONDS=86400\n'
  printf 'LEARNING_BACKUP_RETENTION_DAYS=14\n'
} > "$temporary"
mv "$temporary" "$destination"
echo "created $destination with mode 600"
