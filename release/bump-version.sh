#!/usr/bin/env bash
# release/bump-version.sh — the release version's single surface.
#
#   release/bump-version.sh <new-version>   rewrite every allow-listed location
#   release/bump-version.sh --check         assert every location agrees
#   release/bump-version.sh --print         print the current release
#
# Exit codes: 0 agreed or rewritten, 1 a refusal, 2 a usage error.
#
# The model
# ---------
# `release/make-release.sh`'s `RELEASE=` line is the source of truth. Every other
# place that names the release is a copy of it, and every copy is named below on
# purpose, with the reason it exists.
#
# `--check` proves three separate things:
#
#   1. every coupling — a field a program actually reads, not just prose — equals
#      the source of truth;
#   2. every allow-listed location still exists. A location that quietly stops
#      matching its pattern would quietly stop being bumped;
#   3. the release literal appears nowhere else in the tracked tree. That is the
#      property that matters most: this surface cannot grow in silence, because
#      the next person who hardcodes the release somewhere new fails this check
#      on their own change and either fixes the code or adds an entry here.
#
# Allowlist entry kinds
#   bump     the line names the current release. A bump rewrites the release
#            literal on it.
#   history  the line records a past release. Any past version literal is fine in
#            such a line, the current release's literal is accepted only on a
#            line of this shape, and a bump never rewrites it — it goes on the
#            review list instead, because only a person can write what the new
#            release contains.
#   ignore   the match is not a release version at all.
#
# `history` and `ignore` win over `bump`: a line either of them claims is never
# rewritten, whatever else matches it.
#
# How tight a pattern should be
# -----------------------------
# A pattern that pins the shape of the surrounding sentence turns an editorial
# rewrite into a failed check, for a change that broke nothing. So the shape is
# pinned only where the shape is the contract:
#
#   code and data a program reads   the exact line, and a minimum number of them
#   prose                           any line that names the release; all of them
#                                   are rewritten and none is required
#
# The rewrite proves the prose case as strongly as the pinned one: after a bump
# the scan looks for the *previous* release across the whole tracked tree, so a
# mention it failed to reach is reported by `path:line` either way.
#
# This script names no version literal of its own, so it never appears in its own
# scan.

set -euo pipefail

PROG="$(basename -- "$0")"
REPO="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd -- "$REPO"

SOURCE_FILE='release/make-release.sh'
SOURCE_RX='^RELEASE=(.*)$'
VERSION_RX='^[0-9]+\.[0-9]+\.[0-9]+$'
# The shape of a release-history sentence in STATUS.md. One definition serves the
# allowlist entry and the review list.
HISTORY_RX='^Release `[0-9]+\.[0-9]+\.[0-9]+` '
# What separates a release from a longer dotted number. One definition serves the
# scan and the rewrite, so the two can never disagree about what an occurrence is.
# A release may end a sentence — `… the release page of 0.2.x.` — but it is not an
# occurrence when it sits inside an address (10.0.2.x), inside a longer version
# (0.2.x1), or in front of a fourth component (0.2.x.1).
LEFT_GUARD='(^|[^0-9.])'
RIGHT_GUARD='([^0-9.]|\.[^0-9]|\.$|$)'

FAILED=0
FAILURES=()

die() { printf '%s: %s\n' "$PROG" "$*" >&2; exit 1; }
fail() { FAILURES+=("$*"); FAILED=$((FAILED + 1)); }
note() { printf '%s\n' "$*"; }

# report_failures <headline> — print what disagreed, then refuse.
report_failures() {
	printf '%s: %s\n' "$PROG" "$1" >&2
	printf '  %s\n' "${FAILURES[@]}" >&2
	printf '%s: %d problem(s). Fix them, or add an intentional allowlist entry to release/%s.\n' \
		"$PROG" "$FAILED" "$PROG" >&2
	exit 1
}

usage() {
	cat <<'USAGE'
release/bump-version.sh <new-version>   rewrite every allow-listed location
release/bump-version.sh --check         assert every location agrees with
                                        release/make-release.sh's RELEASE=
release/bump-version.sh --print         print the current release

Exit codes: 0 agreed or rewritten, 1 a refusal, 2 a usage error.
USAGE
}

usage_error() {
	printf '%s: %s\n' "$PROG" "$*" >&2
	usage >&2
	exit 2
}

# rx_literal <text> — the text as an ERE that matches itself. Only the dots of a
# version string need it.
rx_literal() { printf '%s' "${1//./\\.}"; }

# ---------------------------------------------------------------------------
# The couplings: fields a program reads. Each one must appear exactly once, and
# its value must equal the source of truth. The regular expression captures
# whatever value is there, so a mismatch is reported as a mismatch rather than
# as a missing line.
# ---------------------------------------------------------------------------

COUPLINGS=()
couple() { # path regex description
	COUPLINGS+=("$1"$'\t'"$2"$'\t'"$3")
}

# The homepage, the launcher and make-release.sh all read the matrix's release.
couple 'docs/support-matrix.md' '^[[:space:]]*"release": "([^"]*)",$' \
	'the support matrix release field'
# Pinned to the matrix by go/internal/launcher/profile_test.go as well.
couple 'go/internal/launcher/launcher.go' '^const Release = "([^"]*)"$' \
	'the launcher Release constant'
# The homepage is NOT on this list, and adding it back would be a mistake. Its
# download links address GitHub's /releases/latest, so the page carries no
# release number in code, in the deployment configuration, or in its own text.
# Both installers put this value in the enrollment request they send.
couple 'release/kit/Install-BibitesMultiverse.ps1' \
	"^\\\$Release[[:space:]]*= '([^']*)'\$" \
	'the Windows installer $Release'
couple 'release/kit/install-bibites-multiverse.sh' "^RELEASE='([^']*)'\$" \
	'the Linux installer RELEASE'
# What a player reads on the installer window.
couple 'release/kit/Install-BibitesMultiverse-Gui.ps1' \
	"^\\\$form\\.Text = 'Install Bibites Multiverse ([^']*)'\$" \
	'the Windows installer GUI title'
# The install test asserts the enrollment request carries the release.
couple 'release/test-install-uninstall.sh' '"release":"([^"]*)"' \
	'the install test expected enrollment release'

# ---------------------------------------------------------------------------
# The allowlist: path, kind, the least number of lines that must match, the
# pattern, and why the location exists. `@@V@@` stands for the release literal.
# Built by reading every hit of the release literal in the tracked tree.
# ---------------------------------------------------------------------------

ALLOW=()
allow() { # path kind min pattern note
	ALLOW+=("$1"$'\t'"$2"$'\t'"$3"$'\t'"$4"$'\t'"$5")
}

# --- the source of truth ---------------------------------------------------
allow 'release/make-release.sh' bump 1 '^RELEASE=@@V@@$' \
	'the source of truth; every other entry is checked against it'
# make-release.sh names the release nowhere else. Its Linux compile-gate comment
# used to, and no longer does; a comment that names a version again fails the
# scan below, which is the intended answer.

# --- code and data a program reads: the exact line, and how many -----------
allow 'docs/support-matrix.md' bump 1 '^  "release": "@@V@@",$' \
	'the published matrix release; the launcher test and make-release.sh read it'
allow 'go/internal/launcher/launcher.go' bump 1 '^const Release = "@@V@@"$' \
	'the launcher stamps this into its own output and the install record'
allow 'release/kit/Install-BibitesMultiverse.ps1' bump 1 \
	"^\\\$Release[[:space:]]*= '@@V@@'\$" \
	'the Windows installer enrollment release'
allow 'release/kit/install-bibites-multiverse.sh' bump 1 "^RELEASE='@@V@@'\$" \
	'the Linux installer enrollment release'
allow 'release/kit/Install-BibitesMultiverse-Gui.ps1' bump 1 \
	"^\\\$form\\.Text = 'Install Bibites Multiverse @@V@@'\$" \
	'the installer window title'
allow 'release/test-install-uninstall.sh' bump 1 \
	'^check "the request identifies release @@V@@" ' \
	'the install test check name'
allow 'release/test-install-uninstall.sh' bump 1 '"release":"@@V@@"' \
	'the install test expected enrollment payload'
# go/internal/archive/landing.go, go/internal/archive/release.go and their tests
# carry no release literal, which is why none of them appears here. The page's
# two download buttons and its checksum link address /releases/latest, and
# make-release.sh publishes the two stable-named copies those addresses need.
# THE PAGE DOES NOW NAME THE RELEASE, and that changes nothing about this rule:
# the tag is fetched from GitHub's own latest-release endpoint at run time and
# cached in the archive process (release.go), so no build of that page ever
# contains the number. The tests name a deliberately fictional tag for the same
# reason. If a release literal appears in any of those files, the scan below
# reports it and the answer is to take it out, not to add an entry here.
# A worked example of a request the installers actually send; its field shape is
# the contract, so it is pinned like code.
allow 'contracts/public-enrollment.md' bump 1 '^  "release": "@@V@@"$' \
	'the worked enrollment request in the contract'

# --- prose: every mention, whatever sentence carries it --------------------
# These files are rewritten by their owners between releases. Pinning the
# sentence around the version would make an editorial change fail this check
# while the release surface stayed correct, so the pattern is the literal
# itself: any line that names the release is expected, and all of them are
# rewritten. Nothing here is required to exist.
allow 'README.md' bump 0 '@@V@@' \
	'the badge, the callout, the release links and the two platform rows'
allow 'STATUS.md' bump 0 '@@V@@' \
	'the opening line, the Release row and the Homepage row'
# The release-history paragraph, and the reason the prose entry above it is safe:
# a history line is claimed here first and is never rewritten. Past releases stay
# named for ever, and the newest sentence describes what that release changed —
# prose only a person can write, so it goes on the review list instead.
allow 'STATUS.md' history 0 "$HISTORY_RX" \
	'the release-history paragraph; a bump never rewrites it'
allow 'docs/README.md' bump 0 '@@V@@' \
	'the documentation release state'
# The date beside the release on this page is human-owned; the release is not.
allow 'docs/defaults-audit.md' bump 0 '@@V@@' \
	'the audited release; its date is on the review list'
allow 'docs/error-taxonomy.md' bump 0 '@@V@@' \
	'the quoted release, and the commands in the INS-* rows'
allow 'docs/participant/install.md' bump 0 '@@V@@' \
	'the download table and every checksum or Unblock-File command'
allow 'docs/participant/join.md' bump 0 '@@V@@' \
	'the release whose packages carry the public join configuration'
allow 'release/README.md' bump 0 '@@V@@' \
	'the release runbook: artifact names, tag and publish commands'
allow 'release/kit/README-linux.md' bump 0 '@@V@@' \
	'the checksum command in the shipped Linux kit page'

# --- known non-version literals --------------------------------------------
# IPv4 test fixtures. The scan already refuses a match that sits inside a longer
# dotted number, so `10.2.3.4` cannot read as release `2.3.4`; this entry records
# that the file is known and deliberate rather than merely unmatched.
allow 'cloud/aws/test-validation.sh' ignore 0 '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' \
	'RFC 1918 address fixtures, not release versions'
# COMPONENT versions inside worked protocol examples. `modVersion`,
# `sidecarVersion` and `relayVersion` are informational fields a frame carries
# about the program that sent it, and the numbers in these examples were written
# when those programs were at them. They are not this project's release, they do
# not move with it, and a bump that rewrote them would silently edit a ratified
# contract's example frames. They are claimed here so that a release which
# happens to equal one of them — as 0.3.x equals the mod version in
# contract-a.md's CONFIG_UPDATE — is a line somebody has read rather than a
# check nobody can pass.
allow 'contracts/contract-a.md' ignore 0 '^ *"modVersion": "[0-9]+\.[0-9]+\.[0-9]+",?$' \
	'the mod version in a worked CONFIG_UPDATE frame, not the release'
allow 'contracts/contract-b-m3.md' ignore 0 \
	'^ *"(sidecarVersion|relayVersion)": "[0-9]+\.[0-9]+\.[0-9]+",?$' \
	'the sidecar and relay versions in worked handshake frames, not the release'
# The GAME's own version constants, read out of its assembly. m1_findings.md's
# table is a decompile record: every number in it belongs to The Bibites.
allow 'm1_findings.md' ignore 0 'canUpdateFromOlderVersion' \
	"the game's own Utility.Version constants, not this project's release"

# ---------------------------------------------------------------------------
# Reading the tree
# ---------------------------------------------------------------------------

# read_source_release — sets RELEASE and RELEASE_LINE from the source of truth.
read_source_release() {
	local n=0 count=0 line
	[ -f "$SOURCE_FILE" ] || die "$SOURCE_FILE is missing; this is not the repository root"
	RELEASE=''
	RELEASE_LINE=0
	while IFS= read -r line || [ -n "$line" ]; do
		n=$((n + 1))
		if [[ $line =~ $SOURCE_RX ]]; then
			count=$((count + 1))
			RELEASE_LINE=$n
			RELEASE=${BASH_REMATCH[1]}
		fi
	done <"$SOURCE_FILE"
	[ "$count" -eq 1 ] \
		|| die "$SOURCE_FILE has $count lines matching 'RELEASE=', want exactly one"
	[[ $RELEASE =~ $VERSION_RX ]] \
		|| die "$SOURCE_FILE:$RELEASE_LINE: RELEASE='$RELEASE' is not an X.Y.Z version"
}

# check_couplings <release> — every coupling equals the release.
check_couplings() {
	local want=$1 entry path rx desc line n count value lineno
	while IFS=$'\t' read -r path rx desc; do
		[ -n "$path" ] || continue
		if [ ! -f "$path" ]; then
			fail "$path: missing, so $desc cannot be checked"
			continue
		fi
		n=0
		count=0
		value=''
		lineno=0
		while IFS= read -r line || [ -n "$line" ]; do
			n=$((n + 1))
			if [[ $line =~ $rx ]]; then
				count=$((count + 1))
				lineno=$n
				value=${BASH_REMATCH[1]}
			fi
		done <"$path"
		if [ "$count" -eq 0 ]; then
			fail "$path: $desc is gone; nothing matches its pattern"
		elif [ "$count" -gt 1 ]; then
			fail "$path: $desc matches $count lines, want exactly one"
		elif [ "$value" != "$want" ]; then
			fail "$path:$lineno: $desc is '$value', want '$want'"
		fi
	done < <(printf '%s\n' "${COUPLINGS[@]}")
}

# count_matches <path> <pattern> — matching lines, 0 when the file has none.
count_matches() {
	[ -f "$1" ] || { printf '0'; return 0; }
	grep -cE -e "$2" -- "$1" || true
}

# check_allow_counts <release> — every allow-listed location still exists.
check_allow_counts() {
	local vrx path kind min pat notetext n
	vrx="$(rx_literal "$1")"
	while IFS=$'\t' read -r path kind min pat notetext; do
		[ -n "$path" ] || continue
		[ "$min" -gt 0 ] || continue
		if [ ! -f "$path" ]; then
			fail "$path: missing, but the allowlist expects $notetext"
			continue
		fi
		n="$(count_matches "$path" "${pat//@@V@@/$vrx}")"
		if [ "$n" -lt "$min" ]; then
			fail "$path: $n of $min expected lines match '$notetext'; the location moved or was reworded"
		fi
	done < <(printf '%s\n' "${ALLOW[@]}")
}

# scan_literal <version> <mode> — every occurrence of the version literal in the
# tracked tree must be allow-listed. mode 'current' accepts any entry kind, mode
# 'stale' accepts only history and ignore, and is what proves a bump left nothing
# behind.
#
# LEFT_GUARD/RIGHT_GUARD keep a version out of a longer dotted number: a release
# such as 2.3.4 must not match inside the address 10.2.3.4, and a release must
# not match inside a longer version that starts with it.
scan_literal() {
	local ver=$1 mode=$2 vrx scan_rx hits hit path rest lineno text
	local e e_path e_kind e_min e_pat e_note pat
	local matched matched_keep reason
	vrx="$(rx_literal "$ver")"
	scan_rx="${LEFT_GUARD}${vrx}${RIGHT_GUARD}"
	git rev-parse --show-toplevel >/dev/null 2>&1 \
		|| die 'not a git repository; --check needs the tracked file list'
	hits="$(git ls-files -z \
		| xargs -0 -r grep -InE -e "$scan_rx" -- /dev/null 2>/dev/null || true)"
	SCAN_HITS=0
	[ -n "$hits" ] || return 0
	while IFS= read -r hit; do
		[ -n "$hit" ] || continue
		path=${hit%%:*}
		rest=${hit#*:}
		lineno=${rest%%:*}
		text=${rest#*:}
		SCAN_HITS=$((SCAN_HITS + 1))
		matched=0
		matched_keep=0
		for e in "${ALLOW[@]}"; do
			IFS=$'\t' read -r e_path e_kind e_min e_pat e_note <<<"$e"
			[ "$e_path" = "$path" ] || continue
			pat=${e_pat//@@V@@/$vrx}
			[[ $text =~ $pat ]] || continue
			matched=1
			case $e_kind in
			history | ignore) matched_keep=1 ;;
			esac
		done
		if [ "$matched" -eq 0 ]; then
			if [ "$mode" = stale ]; then
				reason="still names the previous release; the bump did not reach it"
			else
				reason="names the release outside the allowlist; add an entry to release/$PROG or remove the literal"
			fi
			fail "$path:$lineno: $reason"
		elif [ "$mode" = stale ] && [ "$matched_keep" -eq 0 ]; then
			fail "$path:$lineno: still names the previous release after the bump"
		fi
	done <<<"$hits"
}

# ---------------------------------------------------------------------------
# The three commands
# ---------------------------------------------------------------------------

do_check() {
	local quiet=${1:-}
	read_source_release
	check_couplings "$RELEASE"
	check_allow_counts "$RELEASE"
	scan_literal "$RELEASE" current
	[ "$FAILED" -eq 0 ] || report_failures \
		"the release surface disagrees with $SOURCE_FILE:$RELEASE_LINE (RELEASE=$RELEASE):"
	[ -n "$quiet" ] || note "release $RELEASE ($SOURCE_FILE:$RELEASE_LINE): $SCAN_HITS allow-listed locations, no stray version literal"
}

# review_at <path> <pattern> — 'path:line', or 'path' when nothing matches.
review_at() {
	local n=''
	if [ -f "$1" ]; then
		n="$(grep -nE -e "$2" -- "$1" 2>/dev/null | tail -n 1 | cut -d: -f1 || true)"
	fi
	if [ -n "$n" ]; then printf '%s:%s' "$1" "$n"; else printf '%s' "$1"; fi
}

# history_newest — the release named on the last release-history line, so the
# review list reports what is actually written there rather than what a previous
# bump assumed.
history_newest() {
	local line
	line="$(grep -E -e "$HISTORY_RX" -- 'STATUS.md' 2>/dev/null | tail -n 1 || true)"
	if [[ $line =~ ([0-9]+\.[0-9]+\.[0-9]+) ]]; then
		printf '%s' "${BASH_REMATCH[1]}"
	else
		printf 'the previous release'
	fi
}

print_review_list() {
	local new=$1
	note ''
	note 'Review by hand — a bump must not write any of these:'
	note "  $(review_at 'docs/defaults-audit.md' 'audit was updated for release')"
	note '      the audit date beside the release. Set it to the day this release is audited.'
	note "  $(review_at 'docs/support-matrix.md' '^  "published":')"
	note '      "published", and both "tested" strings further down. They are claims about'
	note '      what was actually run, so only a person may change them.'
	note "  $(review_at 'STATUS.md' "$HISTORY_RX")"
	note "      the release-history paragraph. Its newest sentence still names $(history_newest)."
	note "      Add one saying what $new changes; the earlier sentences stay as they are."
	note "  $(review_at 'STATUS.md' '^Last updated: ')"
	note '      the "Last updated" date, and the announced service period below it.'
	note "  $(review_at 'release/RELEASE-PAGE.md' '^## What is new in ')"
	note '      "What is new" and "Upgrading from an earlier release". On main each holds a'
	note "      comment and nothing else. WRITE them for $new rather than adding to them: they"
	note '      describe ONE release, and a release has already announced its predecessor'"'"'s'
	note '      whole feature list by being appended to instead of written.'
}

do_rewrite() {
	local new=$1 old vrx subst path script addr before after n
	local e_path e_kind e_min e_pat e_note pat
	local -a paths=() changed=()
	local lines=0

	[[ $new =~ $VERSION_RX ]] || usage_error "'$new' is not an X.Y.Z version"
	read_source_release
	old=$RELEASE
	[ "$old" != "$new" ] || die "the release is already $new ($SOURCE_FILE:$RELEASE_LINE)"

	note "Checking the tree at $old before rewriting it to $new."
	do_check quiet

	vrx="$(rx_literal "$old")"
	# The substitution carries the same guard as the scan, so it rewrites the
	# release and never a longer dotted number that merely contains it: an IPv4
	# address whose last octets read like the release survives a bump of the line
	# it sits on. It runs twice because a match consumes its own boundary
	# characters, which would otherwise leave a second occurrence one character
	# away behind.
	subst="s/${LEFT_GUARD}${vrx}${RIGHT_GUARD}/\\1${new}\\2/g"

	# The files a bump may write, in allowlist order.
	while IFS=$'\t' read -r e_path e_kind e_min e_pat e_note; do
		[ "$e_kind" = bump ] || continue
		[ -f "$e_path" ] || continue
		case " ${paths[*]-} " in
		*" $e_path "*) ;;
		*) paths+=("$e_path") ;;
		esac
	done < <(printf '%s\n' "${ALLOW[@]}")

	for path in "${paths[@]}"; do
		script=''
		# A line a history or ignore entry claims is never rewritten: 'Nb' sends
		# sed to the end of the script for that line before any substitution can
		# reach it. Keep wins over bump, which is what lets one relaxed pattern
		# cover a whole prose file while its release-history sentences stay.
		while IFS=$'\t' read -r e_path e_kind e_min e_pat e_note; do
			[ "$e_path" = "$path" ] || continue
			case $e_kind in history | ignore) ;; *) continue ;; esac
			pat=${e_pat//@@V@@/$vrx}
			while IFS= read -r n; do
				[ -n "$n" ] || continue
				script+="${n}b"$'\n'
			done < <(grep -nE -e "$pat" -- "$path" 2>/dev/null | cut -d: -f1)
		done < <(printf '%s\n' "${ALLOW[@]}")
		while IFS=$'\t' read -r e_path e_kind e_min e_pat e_note; do
			[ "$e_path" = "$path" ] || continue
			[ "$e_kind" = bump ] || continue
			pat=${e_pat//@@V@@/$vrx}
			addr=${pat//\//\\/}
			script+="/$addr/{"$'\n'"$subst"$'\n'"$subst"$'\n'"}"$'\n'
		done < <(printf '%s\n' "${ALLOW[@]}")
		[ -n "$script" ] || continue
		before="$(count_matches "$path" "$vrx")"
		sed -E -i -e "$script" -- "$path"
		after="$(count_matches "$path" "$vrx")"
		n=$((before - after))
		[ "$n" -gt 0 ] || continue
		lines=$((lines + n))
		changed+=("$path")
	done

	note "Rewrote $lines line(s) in ${#changed[@]} file(s):"
	printf '  %s\n' "${changed[@]}"

	note ''
	note "Verifying the tree at $new."
	FAILED=0
	FAILURES=()
	read_source_release
	[ "$RELEASE" = "$new" ] \
		|| die "$SOURCE_FILE:$RELEASE_LINE: the source of truth is '$RELEASE' after the rewrite, want '$new'"
	check_couplings "$new"
	check_allow_counts "$new"
	scan_literal "$new" current
	scan_literal "$old" stale
	[ "$FAILED" -eq 0 ] || report_failures \
		"the rewrite to $new left the tree edited and these locations wrong:"
	note "The tree agrees on $new."
	print_review_list "$new"
}

main() {
	[ $# -eq 1 ] || usage_error 'want exactly one argument'
	case $1 in
	--print)
		read_source_release
		printf '%s\n' "$RELEASE"
		;;
	--check)
		do_check
		;;
	-h | --help)
		usage
		;;
	-*)
		usage_error "unknown option '$1'"
		;;
	*)
		do_rewrite "$1"
		;;
	esac
}

main "$@"
