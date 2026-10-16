#!/usr/bin/env bash
set -euo pipefail
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
CURL="${CURL:-curl}"
command -v python3 >/dev/null || { echo 'python3 is required to read CSRF tokens from HTML' >&2; exit 1; }
read -r -p 'Admin username: ' ADMIN_USERNAME
read -r -s -p 'Admin password: ' ADMIN_PASSWORD
echo
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"; unset ADMIN_PASSWORD' EXIT
COOKIE_JAR="$TMP_DIR/cookies.txt"
extract_csrf() {
  python3 -c '
from html.parser import HTMLParser
import sys
class Parser(HTMLParser):
    token = ""
    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        if tag == "input" and a.get("name") == "csrf_token" and not self.token:
            self.token = a.get("value", "")
p = Parser(); p.feed(sys.stdin.read()); print(p.token)
'
}
expect_code() {
  actual="$1" expected="$2" what="$3"
  if [[ "$actual" != "$expected" ]]; then
    echo "FAIL $what: expected HTTP $expected, got $actual" >&2
    exit 1
  fi
  echo "PASS $what -> HTTP $actual"
}

LOGIN_HTML="$($CURL --fail --silent --show-error -c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/admin/login")"
CSRF="$(printf '%s' "$LOGIN_HTML" | extract_csrf)"
[[ -n "$CSRF" ]] || { echo 'Could not extract login CSRF token' >&2; exit 1; }
LOGIN_CODE="$($CURL --silent --show-error -c "$COOKIE_JAR" -b "$COOKIE_JAR" -o /dev/null -w '%{http_code}' -X POST \
  --data-urlencode "csrf_token=$CSRF" --data-urlencode "username=$ADMIN_USERNAME" --data-urlencode "password=$ADMIN_PASSWORD" "$BASE_URL/admin/login")"
expect_code "$LOGIN_CODE" 303 'admin login'
unset ADMIN_PASSWORD

FORM_HTML="$($CURL --fail --silent --show-error -c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/admin/projects/new")"
CSRF="$(printf '%s' "$FORM_HTML" | extract_csrf)"
[[ -n "$CSRF" ]] || { echo 'Could not extract create-project CSRF token' >&2; exit 1; }
SLUG="curl-acceptance-$(date +%s)"
TITLE="CRUD Acceptance $(date +%H%M%S)"
CREATE_HEADERS="$TMP_DIR/create.headers"
CREATE_CODE="$($CURL --silent --show-error -c "$COOKIE_JAR" -b "$COOKIE_JAR" -D "$CREATE_HEADERS" -o /dev/null -w '%{http_code}' -X POST \
  -F "csrf_token=$CSRF" -F "title=$TITLE" -F "slug=$SLUG" -F 'summary=Created by the curl acceptance script' \
  -F 'description=This temporary item validates create, read, update and delete.' -F 'technologies=Go, Docker' \
  -F 'github_url=https://github.com/example/portfolio-crud' -F 'demo_url=https://example.com' -F 'status=published' \
  "$BASE_URL/admin/projects")"
expect_code "$CREATE_CODE" 303 'create project'
LOCATION="$(awk 'BEGIN{IGNORECASE=1} /^Location:/{gsub("\r", ""); print $2}' "$CREATE_HEADERS" | tail -n1)"
[[ "$LOCATION" =~ ^/admin/projects/([0-9a-fA-F-]+)/edit$ ]] || { echo "Unexpected create redirect: $LOCATION" >&2; exit 1; }
PROJECT_ID="${BASH_REMATCH[1]}"

LIST_BODY="$($CURL --fail --silent --show-error -c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/admin/projects")"
printf '%s' "$LIST_BODY" | grep -Fq "$SLUG" || { echo 'FAIL project missing from admin list' >&2; exit 1; }
echo 'PASS project list'
DETAIL_CODE="$($CURL --silent --show-error -o /dev/null -w '%{http_code}' "$BASE_URL/projects/$SLUG")"
expect_code "$DETAIL_CODE" 200 'published project detail'

EDIT_HTML="$($CURL --fail --silent --show-error -c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/admin/projects/$PROJECT_ID/edit")"
CSRF="$(printf '%s' "$EDIT_HTML" | extract_csrf)"
[[ -n "$CSRF" ]] || { echo 'Could not extract update CSRF token' >&2; exit 1; }
UPDATE_CODE="$($CURL --silent --show-error -c "$COOKIE_JAR" -b "$COOKIE_JAR" -o /dev/null -w '%{http_code}' -X POST \
  -F "csrf_token=$CSRF" -F "title=$TITLE Updated" -F "slug=$SLUG" -F 'summary=Updated by the curl acceptance script' \
  -F 'description=Update operation checked with curl.' -F 'technologies=Go, PostgreSQL' \
  -F 'github_url=https://github.com/example/portfolio-crud' -F 'demo_url=https://example.com' -F 'status=published' \
  "$BASE_URL/admin/projects/$PROJECT_ID")"
expect_code "$UPDATE_CODE" 303 'update project'
EDIT_HTML="$($CURL --fail --silent --show-error -c "$COOKIE_JAR" -b "$COOKIE_JAR" "$BASE_URL/admin/projects/$PROJECT_ID/edit")"
CSRF="$(printf '%s' "$EDIT_HTML" | extract_csrf)"
DELETE_CODE="$($CURL --silent --show-error -c "$COOKIE_JAR" -b "$COOKIE_JAR" -o /dev/null -w '%{http_code}' -X POST \
  -F "csrf_token=$CSRF" "$BASE_URL/admin/projects/$PROJECT_ID/delete")"
expect_code "$DELETE_CODE" 303 'delete project'
AFTER_DELETE="$($CURL --silent --show-error -o /dev/null -w '%{http_code}' "$BASE_URL/projects/$SLUG")"
expect_code "$AFTER_DELETE" 404 'deleted project is no longer public'
echo 'PASS CRUD create/list/detail/update/delete acceptance'
