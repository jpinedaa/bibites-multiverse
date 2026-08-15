# Handoff — registering `bibitesmultiverse.com`

**For a session with no memory of the conversation that produced this file.** It is a single
errand: get one domain name registered, in the owner's browser, with the owner present. It is
written on 2026-08-14 against commit `5d5c19c`, and it assumes you know nothing about this
project beyond what is here and what is behind the paths named here.

The errand is small and it is not reversible. The name is minted into every participant's join
string and this project's release channel pushes nothing, so a name changed after strangers have
joined is a message the owner has to deliver to people he cannot reach
(`wp3_hosting_options.md`, *TLS, DNS, and what the name costs to change later*, and §"Why this
name and not another" below). Register the right name, then stop where §6 tells you to stop.

---

## 1. Who does what, and the rule that matters

The owner asked for this shape in his own words: *"can you control the browser? go to purchase
and tell me when i need to fill like passwords or credit card otherwise you fill everything."*
That is the contract. Read it as two lists and one rule.

**You do:** open and drive the browser, read every page before acting on it, type the domain name
into the search box, fill contact and registrant fields from values the owner gives you, set
checkbox and radio states, take screenshots so the owner can see what you see, and narrate where
you are in the flow.

**The owner does, personally, at the keyboard:** every password, every payment detail (card
number, expiry, CVC, billing card name), every 2FA or email-verification code, and **the final
click that spends money**. Nothing on that list ever passes through you.

**The rule: announce before you hand over, never wait silently.** Before each handover, say in
one short message (a) what the next field or button is, (b) why it is his and not yours, and
(c) what you will do the moment he tells you he is done. A session that stops without saying why
looks broken; a session that stops with a sentence looks like the plan.

**Secrets are never recorded.** Do not put a password, a card number, a CVC, a 2FA code or a
one-time link into a tool argument, a file, a commit, a screenshot description, or your final
report. If the owner pastes one into the chat by accident, do not repeat it back and do not use
it in a `fill` call — tell him to type it into the browser himself. `take_screenshot` on a page
with a filled card field is a recording of a card number; do not take one.

**Driving Chrome.** The `chrome-devtools` tools are deferred, so load their schemas first:

```
ToolSearch: select:mcp__chrome-devtools__list_pages,mcp__chrome-devtools__new_page,mcp__chrome-devtools__navigate_page,mcp__chrome-devtools__take_snapshot,mcp__chrome-devtools__click,mcp__chrome-devtools__fill,mcp__chrome-devtools__fill_form,mcp__chrome-devtools__take_screenshot,mcp__chrome-devtools__wait_for
```

Then: `list_pages` first to see what is already open — this is **the owner's browser with the
owner's sessions in it**, not a scratch profile, so do not close pages you did not open and do
not navigate a tab out from under him. `take_snapshot` before every `click` or `fill`, because
element uids are per-snapshot and a stale uid will click the wrong thing on a page that is
partly about money. `wait_for` on expected text rather than sleeping. When something looks
different from what this document describes, believe the page and not this document, say so, and
ask.

---

## 2. Why this name and not another

The name is decided and is already threaded through the deployment kit. `deploy/deploy.env.example`
line 35 reads `MV_DOMAIN=bibitesmultiverse.com` and line 41 reads
`MV_CERT_EXTRA_NAMES=status.bibitesmultiverse.com`, so both public names are on nginx's
certificate. `deploy/provision.sh` refuses to run on a
placeholder domain at all (`phase_preflight`, lines 116–120) with the reason stated inline: the
name is baked into every join string the relay ever mints.

The join string is literally `multiverse-join/1 wss://<host>/contract-b/v4 <peerId>.<secret>`
(`wp3_hosting_options.md`, *What changing the name costs*, which cites
`go/internal/peercred/peercred.go:485` and `relay.go:63`). Every participant's sidecar holds that
URL in its configuration, and the release channel — GitHub Releases — pushes nothing to anybody.
Hence the standing instruction in that document: **treat the name as permanent for the run.**

**Why this name and not one of the others.** It came off a shortlist of seven verified-available
names in three families, and it was picked as the most literal and most searchable of them, at the
cost of ten more characters in every join string. The shortlist, the RDAP method behind it, the
names already known to be taken, and the trademark position — *"The Bibites"* is the developer's
mark, and the owner decided on 2026-08-12 not to ask him — are all in `wp3_hosting_options.md`,
*Which registrar to buy from, and which name to buy*. If the owner reopens the question mid-flow,
that is the section to take him back to, along with §9E below.

`status.bibitesmultiverse.com` is on the certificate because a second name costs nothing at
issuance and cannot be added later without a re-issue. You do not need to do anything about it
today beyond knowing it exists and will need its own A record later.

---

## 3. Registrar: Cloudflare, with Porkbun as the fallback

| | Cloudflare Registrar | Porkbun (fallback) |
|---|---|---|
| `.com` per year | **~$10.44** — registry cost plus the $0.18 ICANN fee **[web, 2026-08-14]** | **$11.08**, all fees included **[web, 2026-08-14]** |
| Renewal | **The same price.** Cloudflare sells at cost, so there is no introductory rate that jumps | Same price at renewal |
| WHOIS privacy | **Free and automatic** (Cloudflare's own words: free WHOIS redaction). There is no upsell to decline | Free |
| Nameservers | **Cloudflare's own, mandatory.** You cannot delegate the zone elsewhere | Free choice |
| Requires an account | Yes — the registrar lives inside the Cloudflare dashboard | Yes |

**Confirm the price on the screen before the owner pays.** The figures above were fetched from
the web on 2026-08-14 and re-verified the same day in `wp3_hosting_options.md`, *Which registrar
to buy from, and which name to buy*, which now carries the full four-way comparison and the same
numbers. They are still web prices and not an order screen. If the screen says something
materially different, say so and let the owner decide.

**One number from that comparison is worth saying out loud at the order screen.** Cloudflare's
flat renewal means *no registrar markup*, not a frozen price: Verisign has announced a `.com`
wholesale rise from $10.26 to **$10.97 effective 2026-11-01**, which an at-cost registrar passes
straight through (~$11.15). It falls after the announced run ends and changes nothing today.

**The nameserver lock-in is real and today it costs nothing.** Registrar and DNS become one
account, and there is nothing to migrate: no zone exists, no record exists, nobody resolves this
name yet. Say that to the owner rather than letting him discover it — it is a genuine constraint
that happens to be free on this particular day.

**One thing it buys later, and one thing that is not your errand.** `deploy/README.md` §4 says
ACME DNS-01 is *"the better answer and it is not available yet: it needs a certbot plugin for a
registrar the owner has not chosen"*, and `provision.sh` refuses `MV_ACME_MODE=dns` rather than
guessing a provider. Registering at Cloudflare makes that plugin choosable later. It is not part
of this handoff, it needs an API token (a secret), and nothing about it should be set up today.

**Where the full argument lives.** `wp3_hosting_options.md`, *Which registrar to buy from, and
which name to buy*, holds the four-way comparison — Cloudflare, Porkbun, Namecheap, Route 53 —
with prices, renewals, the nameserver coupling and why each of the other two loses. That
subsection was written on 2026-08-14, after an earlier version of this handoff correctly reported
that no such comparison existed; it exists now, and it is the authority you may cite to the
owner. This section is its summary, not a second opinion — if the two ever disagree, that
document is the one that gets corrected.

---

## 4. Pre-flight, before you open a browser

**1. Re-verify that the name is still available.** It was available at Verisign's RDAP service on
2026-08-14, and re-confirmed the same day while this document was written. A `.com` can be taken
by anyone at any minute, so check again rather than discovering it at the payment step:

```bash
curl -s -o /dev/null -w "%{http_code}\n" https://rdap.verisign.com/com/v1/domain/bibitesmultiverse.com
```

- **`404` — available.** Proceed.
- **`200` — taken.** **Stop.** Do not start a purchase flow. Go to §9, failure mode A.
- **Anything else** (`429`, `5xx`, a timeout) is RDAP having a bad moment and is not an answer.
  Retry once or twice; if it will not settle, say so and let the registrar's own search page be
  the second opinion — but treat a registrar's "available!" banner as a sales claim rather than a
  fact until the order screen agrees.

**2. Confirm the kit still names it.** `grep -n '^MV_DOMAIN=\|^MV_CERT_EXTRA_NAMES=' deploy/deploy.env.example`
must return `bibitesmultiverse.com` and `status.bibitesmultiverse.com`. If it does not, something
changed after this document was written — surface the difference to the owner and do not buy
either name until he says which one is current.

**3. Ask the owner two questions before the browser opens**, because the answers change the path:

- **"Do you already have a Cloudflare account?"** If yes, you skip §5 step 1 entirely and he only
  has to be signed in. If no, account creation comes first and it involves a password and an
  emailed verification link — both his.
- **"Is 2FA on that account set up, and do you have the device?"** Cloudflare will ask for a code
  during sign-in if it is on, and hunting for a phone mid-purchase is how a flow times out.

**4. Tell him what he will need at hand** before you start, in one message: the email address for
the account, a password he is willing to type, access to that inbox, a payment card, and the 2FA
device if there is one. This is the last comfortable moment to find out the card is in another
room.

---

## 5. The walkthrough

Each step says what you do and what you hand over. UI labels are what Cloudflare's flow looked
like as of the writing of this document and **have not been verified against the live site** —
read each page with `take_snapshot` and follow what it actually says.

**Step 1 — the account (skip if he already has one).**
You: open a page at `https://dash.cloudflare.com/sign-up`, snapshot it, and fill the email
address the owner dictates.
**Hand over:** the password field. Say it plainly — *"the password box is yours; I don't type or
store passwords. Tell me when it's in and I'll continue."* Then the sign-up button is his too,
since it commits the password.
**Hand over:** the verification email. Cloudflare sends a link; the owner opens his own inbox and
clicks it. You do not need to see the inbox and should not ask to.

**Step 2 — sign in.**
You: navigate to `https://dash.cloudflare.com/`.
**Hand over:** the password field, and the 2FA code if prompted. Wait for his word, then
`take_snapshot` to confirm you are on the dashboard.

**Step 3 — find the registrar.**
You: from the dashboard, go to **Domain Registration → Register Domains** (the left-hand
navigation; on some accounts it is reached from the account home rather than a zone). If the
labels differ, snapshot and read. Screenshot it for the owner if you are unsure you are in the
right place — one screenshot is cheaper than one wrong purchase.

**Step 4 — search for the name.**
You: type `bibitesmultiverse.com` into the search field and submit. This is exactly the kind of
field that is yours: it is not a secret, and a typo here is the single most expensive mistake
available in this errand. **Read the result back character by character before going further** —
`bibitesmultiverse.com`, one word, no hyphen, no `s` on `multiverse`, `.com`.

**Step 5 — confirm the price and the term.**
You: read the order screen and report to the owner: the exact name, the yearly price, the term
being bought, and the renewal price shown. Expect the renewal to equal the registration price;
if it does not, that is news and it is worth pausing on.
Term is the owner's call. One year is the smallest commitment and covers the announced
three-month run four times over; multiple years cost less per year in nothing but effort saved.
Ask, do not choose for him.
Note for the owner, unprompted: **WHOIS privacy is free and automatic at Cloudflare**, so if you
see no privacy upsell, nothing is missing. Cloudflare's own documentation also states that
**renewals are final and not refunded**, which is worth him knowing before he picks a long term.

**Step 6 — registrant and contact details.**
You: fill exactly what the owner dictates — name, address, phone, email. These are not secrets in
the sense that matters here, but they are his personal data: type what he gives you, do not
invent, do not autofill from anything you found elsewhere, and do not guess a postcode. If a
field is required and he has not given you a value, ask.
Cloudflare redacts these from public WHOIS automatically; the registry still receives them, which
is how domain registration works everywhere, and he should hear that from you rather than assume
the redaction is anonymity.

**Step 7 — payment details.**
**Hand over, entirely.** Announce it before the page appears if you can see it coming: *"the next
screen is card details. All of it is yours — number, expiry, CVC, billing address on the card. I
won't read them back or screenshot the page. Tell me when it's filled and I'll re-read the order
summary."*
Do not take a screenshot of a page with card fields on it, filled or empty.

**Step 8 — the purchase.**
You: before he clicks, read the order summary aloud one last time — name, term, total. That is
your last useful act in the flow.
**Hand over:** the button that completes the purchase. **The owner clicks it.** Not you. Say why
once: it is his money, and a spend is not a thing an agent should perform on his behalf.
Then wait, and `take_snapshot` after he says it went through.

---

## 6. Stop here. Do not create DNS records.

**This is a stop-rule, not advice.** Once the domain is registered, the errand is over.

The reason is ordering: an A record must point at an address that will not move, and the address
does not exist yet. `deploy/README.md` §6 makes the sequence explicit — step 5 is *"Attach a
static IP … **Do this before the A record**: a Lightsail instance's default public address
changes when it is stopped and started"*, and only step 6 is the record. Pointing the name at a
dynamic address now buys nothing and creates a stale record that will fail ACME issuance later in
a way that looks like a certificate problem.

The two records that will be needed — `bibitesmultiverse.com` and `status.bibitesmultiverse.com`,
both A, both TTL 300 during setup — belong to the Lightsail handoff (§10).

**And the mistake that costs an afternoon, recorded here because it is a property of the account
you just created and the next session will meet it:** Cloudflare turns its proxy **on** by default
for new A records — the orange cloud. **Every record for this project must be "DNS only", the
grey cloud.** Three reasons, all of them in this repo:

- **ACME issuance fails.** `deploy/README.md` §4 fixes issuance at **HTTP-01 through nginx on
  port 80** — DNS-01 is deliberately unavailable, and port 80 is open in the firewall precisely
  to serve one directory of challenge files. A proxied record resolves to Cloudflare's anycast
  address, so the challenge is answered by the wrong host. `provision.sh`'s preflight names this
  exact failure when the resolved address does not match the instance: *"the HTTP-01 challenge
  will be answered by the OTHER host and issuance fails"* (lines 147–160).
- **The `wss://` path changes origin.** nginx terminates origin TLS on port 443
  and proxies the relay path. A Cloudflare proxy would terminate the client
  connection before the configured origin.
- **It is invisible from inside the box.** `provision.sh` pins the domain to `127.0.0.1` in
  `/etc/hosts` (line ~392) so the archive can dial the relay by name, and `README.md` §7 states
  that external reachability *cannot* be tested from the instance at all. So every on-box check
  can pass while no stranger can connect. That is why this is written down instead of left to be
  noticed.

If Cloudflare's UI ever offers a proxy toggle during registration itself, set it off, and say so.

---

## 7. Immediately after the purchase: verify, then stop

Four checks. The first is the only one that proves anything to the outside world.

**1. RDAP now answers.** Rerun the pre-flight command; it should return **`200`** instead of
`404`. Registry propagation is normally seconds to a few minutes, so retry for a couple of minutes
before treating a `404` as a problem.

**2. The registrar is the one that was paid.** Read the record rather than trusting the number:

```bash
curl -s https://rdap.verisign.com/com/v1/domain/bibitesmultiverse.com | head -c 2000
```

You are looking for the registrar entity — Cloudflare, or Porkbun if the fallback was used.

**3. The nameservers are the registrar's.**

```bash
dig +short NS bibitesmultiverse.com
```

Cloudflare assigns two of its own (`*.ns.cloudflare.com`). Empty output right after purchase means
delegation has not propagated yet, not that something is wrong; give it a few minutes. If
`dig` is not installed, `getent hosts` proves nothing useful here — say the check could not be run
rather than substituting one that does not answer the question.

**4. The zone exists in the dashboard.** Cloudflare normally creates the zone on the free plan
automatically when it registers a name. Snapshot the DNS page and confirm the zone is listed —
**and confirm the record list is empty, which is the correct state today.**

**What not to do:** no A records, no AAAA records, no proxy toggles, no email routing, no
redirects, no page rules, no API tokens. The next thing that touches this zone is the Lightsail
handoff, after a static IP exists.

---

## 8. What to record in the repo

One row, one file. The receiving session updates `m5_tracking.md`'s **WP3** row and nothing else.
That document's own rule is that it is *"updated by integration rather than by appending"* — so
edit the sentences that are now wrong rather than adding a new sentence beside them. Two edits:

**Edit 1 — the name sentence.** It currently ends *"…are set in `deploy/deploy.env.example`) — the
owner registers it; it is permanent once the first join string is minted."* Replace the clause
after the parenthesis with:

> — **registered by the owner YYYY-MM-DD at <registrar>**; it is permanent once the first join
> string is minted. No DNS records exist yet: the A records for `bibitesmultiverse.com` and
> `status.bibitesmultiverse.com` wait on the Lightsail static IP (`deploy/HANDOFF-lightsail.md`).

**Edit 2 — the remaining-work list.** It currently reads *"**Remaining**: the domain name; the
owner's 12 manual steps; then provisioning and the announcement"*. The domain name is no longer
remaining:

> **Remaining**: the owner's manual steps from `deploy/README.md` §6, of which step 6 (the domain)
> is now done; then provisioning and the announcement

Fill `YYYY-MM-DD` with the actual purchase date and `<registrar>` with the one that was actually
paid. Do not write the price, the account email, or any order number into the repo — none of it is
tracking state and the last two are personal data.

**Agents do not commit in this project.** Make the edit, leave it in the working tree, and report
the changed path and a proposed commit message to the orchestrator. Do not run `git add`,
`git commit`, or `git push`.

---

## 9. Failure modes

**A. The name is taken (RDAP returns `200`).**
Stop before any purchase. Tell the owner plainly: the name is gone, and here is what the RDAP
record says about when it was registered and by whom, if it says anything. Then go to the
shortlist — **there is one**, in `wp3_hosting_options.md`, *The name: how the shortlist was built,
and what the method cannot tell you*. As of 2026-08-14 it lists six other names verified available
at their registries: `bibiteverse.com`, `bibiteworlds.com`, `evoworlds.com`,
`organismcrossing.com`, `bibites.org` and `bibites.world` — the last two on TLDs whose renewal
economics differ, and the two concept names carrying none of the trademark exposure that document's
*The name the project does not own* describes. **Re-run RDAP against every candidate before
offering it**, because that list has the same shelf life as the check in §4:
`curl -s -o /dev/null -w '%{http_code}\n' https://rdap.verisign.com/com/v1/domain/<name>.com`,
`404` available and `200` taken. The choice is still the owner's, and whatever he picks has to be
written into `deploy/deploy.env.example` lines 35 and 41 — **both**, since the second is
`status.<domain>` — before anything else in the kit is run.

**B. Cloudflare refuses to sell to a brand-new account.**
This happens: registrars gate new or unverified accounts, and Cloudflare's own documentation notes
that an unverified email address causes trouble with registrar operations. First, the cheap fix —
make sure the verification link in the owner's inbox has actually been clicked, and that a payment
method has been added and accepted in Billing. If it still refuses, do not fight it and do not try
a second Cloudflare account. **Fall back to Porkbun** (`porkbun.com`), same name, ~$11.08/yr, free
WHOIS privacy, and **no nameserver lock-in** — which means the later DNS story is Porkbun's own
nameservers rather than Cloudflare's, and the grey-cloud warning in §6 simply does not apply, since
there is no proxy to leave on. Everything else in this document is unchanged; the flow shape is the
same and the handovers are the same.

**C. The payment declines.**
Say what the page says, and nothing more — do not speculate about the card. Common causes are a
bank flagging an unfamiliar merchant and a billing address mismatch. Both are the owner's to
resolve, usually with his phone. Wait; do not retry the submission yourself, and above all do not
retry it repeatedly, since duplicate authorisations are a real outcome.

**D. 2FA is not set up, or the device is not to hand.**
Cloudflare may prompt to enrol during sign-up. Enrolment is entirely the owner's — the secret, the
QR code, the recovery codes, all of it. Never photograph or transcribe a recovery code. If he does
not want to do it now and Cloudflare permits skipping, skipping is legitimate; if the account
already has 2FA and the device is elsewhere, pause the errand rather than working around it.

**E. The owner changes his mind about the name mid-flow.**
Stop the purchase. This is the one decision in the project that cannot be undone cheaply, and a
name typed into a search box under time pressure is exactly how the wrong one gets bought. Take him
back to §2's argument, re-verify the new candidate with RDAP, and if he still wants it, change
`deploy/deploy.env.example` lines 35 and 41 **first**, so the repo and the purchase agree. A name
bought that the kit does not know about is a trap for the next session.

**F. The flow does something this document did not predict.**
Believe the page. Snapshot it, screenshot it if there is nothing sensitive on it, describe what you
see, and ask. This document was written without access to the live registrar UI and says so.

---

## 10. What happens next

Not in this session. The domain is one of the owner's twelve manual steps, listed in full at
`deploy/README.md` §6 — step 6 there is the one you just did, and steps 4, 5 and 7 (create the
Lightsail instance, attach the static IP, open the three firewall ports) are what has to happen
before the DNS records this document forbade you to create.

That work has its own handoff at **`deploy/HANDOFF-lightsail.md`**, written in parallel with this
one. If that file is not present when you look, `deploy/README.md` §6 is the authority and nothing
is lost.

After the instance and the static IP exist, the A records go in — `bibitesmultiverse.com` and
`status.bibitesmultiverse.com`, both **DNS only / grey cloud**, TTL 300 — and only then does
`provision.sh --dry-run` have anything true to check.

---

## 11. What this document does not know

Stated rather than left to be discovered, in the style of `deploy/README.md` §7.

- **The live registrar UI was never opened.** Every label, menu name and page order in §5 is
  expectation. The snapshot on your screen outranks all of it.
- **The prices are from the web on 2026-08-14**, and they are also in `wp3_hosting_options.md` as
  of that date — but not from an order screen. Confirm before the owner pays.
- **The registrar comparison and the name shortlist do exist**, both in `wp3_hosting_options.md`,
  *Which registrar to buy from, and which name to buy*, written 2026-08-14. Earlier revisions of
  this handoff said they did not; that was true when it was written and is not true now.
- **The shortlist's availability was verified on 2026-08-14 and nothing else.** A `404` from RDAP
  does not rule out a registry-reserved or premium-priced name, and §9A still has you re-check.
- **Nothing here has been executed** except the RDAP availability check, which returned `404` on
  2026-08-14 — and that answer has a shelf life measured in minutes, which is why §4 makes you run
  it again.
