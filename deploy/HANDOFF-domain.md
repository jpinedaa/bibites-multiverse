# Domain and DNS handoff

This checklist applies to a new public deployment.
It does not identify the domain, registrar, account, or address of a live service.

## Required result

The deployment needs one stable service name.
That name appears in participant join strings and TLS certificates.
Treat it as permanent for the announced service period.

Complete the purchase and DNS work with the account owner present.
The owner must enter passwords, verification codes, payment details, and purchase approval.
Do not record these values in a terminal, screenshot, issue, or repository.

## Select the name and registrar

Before a purchase, make sure that the selected name meets these requirements:

- The name is easy to read and type.
- The operator can renew it through the full service period.
- The DNS provider supports stable A or AAAA records.
- The operator understands renewal prices and nameserver restrictions.
- The name does not depend on one compute instance.

Registrar prices and availability change often.
Use the current order screen as the source for price and terms.
Do not use an old comparison table as a purchase quote.

Record the registrar, renewal date, and domain owner in private operations storage.
Do not record account email addresses, order numbers, or payment details in Git.

## Order of operations

Use this order:

1. Register the selected name.
2. Create the compute instance.
3. Attach a stable public address to the instance.
4. Create DNS records that point to the stable address.
5. Check the records from outside the instance.
6. Run the provisioning dry run.
7. Request the production certificate.

Do not create a record for an address that can change after an instance stop.
An incorrect record can consume an ACME validation attempt.

## DNS records

Create an A record for `MV_DOMAIN`.
Create records for each name in `MV_CERT_EXTRA_NAMES` only when the deployment uses them.

If the DNS provider offers an HTTP proxy, disable it for the direct-origin deployment.
The default nginx and WebSocket design expects clients and ACME to reach the origin.

If an AAAA record exists, it must reach the same service over IPv6.
Delete an incorrect AAAA record before certificate issuance.

Use a short TTL during initial setup.
Increase the TTL after the address and certificate are stable.

## Checks

Run these checks from a machine outside the instance:

```sh
dig +short A <service-domain> @1.1.1.1
dig +short AAAA <service-domain> @1.1.1.1
dig +short NS <service-domain>
```

The A result must equal the stable public address.
The AAAA result must be empty or must identify the correct IPv6 address.

Then check the public endpoints:

```sh
curl -fsS https://<service-domain>/healthz
curl -sS -o /dev/null -w '%{http_code}\n' \
  https://<service-domain>/contract-b/v4
```

An unauthenticated relay request must return the configured refusal response.
It must not reach a different website or proxy error page.

## Handoff record

Store these non-secret facts in private operations storage:

- The domain and registrar.
- The registration and renewal dates.
- The DNS zone owner.
- The record names, types, targets, and proxy state.
- The certificate names and expiry date.
- The public commit that defines the nginx and provisioning behavior.

Do not add these live values to this public checklist.
