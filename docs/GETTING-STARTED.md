# Getting Started

Clonarr is a visual TRaSH Guides sync tool for Radarr and Sonarr. Here is the quick path from a fresh install to a synced profile.

## 1. Add an instance

Open **Settings > Instances** and add your Radarr or Sonarr: a name, the URL, and the API key (found in Radarr/Sonarr under Settings > General). Click **Test** to check the connection, then save. Repeat for each instance. The app switcher at the top lets you move between them.

## 2. Sync a profile

Go to **Profiles > TRaSH Profiles**, pick the profile you want, and click **Use profile**. Review and configure it, then **Apply and sync**. The profile is created in your Radarr/Sonarr with all its custom formats, scores, and quality settings.

## 3. Customize a profile (optional)

To change anything before syncing, click **Customize this profile**. From there you can add or remove custom formats, change scores, and adjust quality items, then **Apply and sync**.

To use a custom format that is not part of the guide, first create or import it on the **Custom Formats** tab, then add it to a profile under **Customize this profile > Additional CF**.

## Keeping profiles up to date

**Auto-sync** (Settings > Auto-sync) keeps profiles current when the guide changes, or when someone edits the profile directly in Radarr/Sonarr (drift). Pick what happens when a change is found: **Apply automatically** (default) syncs right away, **Just notify me** sends a notification so you can review and apply yourself, and **Wait before applying** delays the apply by a duration you choose. Rules with auto-sync turned off still get an "updates available" flag. You can also pause auto-sync per instance.
