# Homepage widget

Show Clonarr stats on your [gethomepage](https://gethomepage.dev) dashboard.

> **Available on `:dev` and `:preview` only.** Will ship to `:latest` with the next release.

![Clonarr widget in homepage](images/homepage-widget.png)

## What you need

1. Your Clonarr address (e.g. `http://192.168.1.10:6060`)
2. Your Clonarr **API key** — copy it from **Settings → API**

## Add to your `services.yaml`

Replace `CLONARR_URL` and `YOUR_API_KEY` with your own values:

```yaml
- TRaSH Automation:
    - Clonarr:
        icon: https://raw.githubusercontent.com/ProphetSe7en/clonarr/main/ui/static/icons/clonarr.png
        href: CLONARR_URL
        widget:
            type: customapi
            url: CLONARR_URL/api/widget/summary
            method: GET
            headers:
              X-Api-Key: YOUR_API_KEY
            refreshInterval: 30000
            display: block
            mappings:
              - field: { instances: total }
                label: Instances
              - field: { rules: total }
                label: Profiles
              - field: { trash: nextPull }
                label: Next pull
                format: relativeDate
                defaultValue: "Off"
              - field: { autoSync: nextSync }
                label: Next sync
                format: relativeDate
                defaultValue: "Off"

    - Radarr Profiles:
        widget:
            type: customapi
            url: CLONARR_URL/api/widget/summary
            headers:
              X-Api-Key: YOUR_API_KEY
            refreshInterval: 30000
            display: block
            mappings:
              - field: { rules: radarrTotal }
                label: Total
              # One row per profile you want to show. Add or remove rows as needed.
              - field: { rules: { radarrList: { 0: arrProfileName } } }
                label: " "
              - field: { rules: { radarrList: { 1: arrProfileName } } }
                label: " "

    - Sonarr Profiles:
        widget:
            type: customapi
            url: CLONARR_URL/api/widget/summary
            headers:
              X-Api-Key: YOUR_API_KEY
            refreshInterval: 30000
            display: block
            mappings:
              - field: { rules: sonarrTotal }
                label: Total
              # One row per profile you want to show. Add or remove rows as needed.
              - field: { rules: { sonarrList: { 0: arrProfileName } } }
                label: " "
              - field: { rules: { sonarrList: { 1: arrProfileName } } }
                label: " "
```

## Showing more (or fewer) profiles

Each `radarrList`/`sonarrList` row shows **one** profile. Counting starts at `0`:

- 1 profile  → keep just `0`
- 2 profiles → `0` and `1`
- 3 profiles → `0`, `1`, `2`
- and so on

If you list a number higher than how many profiles you actually have, that row just stays empty — nothing breaks.

## Keep your API key private

The Clonarr API key gives **full read and write access** to your install. Treat it like a password — never commit `services.yaml` to a public git repo with your real key in it.

## Verify it works

From a terminal:

```bash
curl -H "X-Api-Key: YOUR_API_KEY" CLONARR_URL/api/widget/summary
```

You should get a long JSON response. If you get:

- `401 Unauthorized` — wrong API key
- `404` — your Clonarr is on `:latest`. Upgrade to `:dev` or `:preview`.
- Connection refused — wrong URL or port.
