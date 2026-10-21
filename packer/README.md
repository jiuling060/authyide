# Ad network catalog

This directory is the single source of truth for the ad networks a user may
attach to one of their apps in the user center.

The backend reads `ad_networks.yaml` at startup, keeps the list in memory, and
exposes it through `GET /api/v1/users/apps/ad-networks`. The frontend renders
its dropdown entirely from that endpoint and hard-codes no network names, so the
UI and the server can never disagree about what is offered.

If the file is missing or fails to parse the backend falls back to a built-in
default list and logs a warning. A bad catalog never blocks the user center.

## `ad_networks.yaml`

```yaml
- id: admob
  displayName: Google AdMob
  vendor: Google
  platforms:
    - android
    - ios
```

| Field         | Meaning                                                                 |
| ------------- | ----------------------------------------------------------------------- |
| `id`          | Stored on the app row as `ad_network`; validated against this list.     |
| `displayName` | Shown in the user center dropdown.                                      |
| `vendor`      | Owner of the network, shown under the display name.                     |
| `platforms`   | Which app platforms the network supports, used to mark an ID unusable.  |

The API serves the same entries with `display_name` (snake_case JSON), which is
what the frontend reads.

`id: none` is reserved for "no ads" and always present; it takes no app id.

## What this catalog decides, and what it does not

The catalog constrains the choice of network. The backend never validates the
format of the user-supplied app id, never contacts an ad platform, never
serves or settles ads, and never ships an SDK. Those responsibilities stay on
the user's own account with the network vendor.

Apps themselves are managed through `/api/v1/users/apps`; the ad network and
app id are fields on the app, editable from the ads tab of the user center
settings dialog.
