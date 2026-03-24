SQP-3 Audio (2160p) - UHD Remux|IMAX-E
This is the start of the Guide.

Many people think that the SQP 3 prefers audio over HDR formats. But that's not true if you follow the guide 100%. For those with an audio setup that supports HD audio and prefer HD audio, I've made the following SQP-3 that will actually prefer HD audio. This also means the following: it will prefer 1080p remuxes over 2160p WEB-DL, except if the 2160p WEB-DL is a hybrid and has HD audio.

Why choose this quality profile
You got a decent audio setup. (that supports all HD audio formats)
You got a setup that completely supports DoVi from start to end.
HDR/DoVi (Depending on what's offered and often both)
HD Audio (Atmos, TrueHD etc...)
You want the highest quality possible, with the option to upgrade to IMAX Enhanced.
-
Workflow Logic
Depending on what's released first and available, the following Workflow Logic will be used:

1080p Remux
When the 4k WEB-DL is released and it's a hybrid and has HD audio, it will download the 4k WEB-DL. (streaming services)
When the 4k Remux is released, it will upgrade to the 4k Remux.
When the IMAX-E is released, it will upgrade to the IMAX-E. (optional, see below)

Possible Variables
When no 4k release exists, it will grab the following:

1080p Remux
1080p WEBDL with DV/HDR (optional also 1080p WEBDL without DV/HDR)

:arrow: "[Optional] IMAX Enhanced (IMAX-E)

When an IMAX Enhanced exists, it will upgrade/downgrade to IMAX Enhanced.
IMAX Enhanced will ONLY be chosen if it has the same AUDIO and HDR Metadata
It won't downgrade from a TrueHD Atmos to a DD+ Atmos or from a DV to an HDR."

:alert: Important Notice :alert:
All the scores and combinations of Custom Formats used in this Guide are tested to achieve the desired results while preventing download loops as much as possible.

From experience, when people change scores or leave out certain CFs that work together, they end up with undesired results.

If you're unsure or have questions, do not hesitate to ask for help on Discord

:alert: Instructions :alert:
Follow every step below.
Don't skip any steps.
Changing the tested recommended scores could result in undesired results.
Adding CF not in this guide could result in undesired results.
-
Create a new Quality Profile
Settings => Profiles

Create a new profile and name it: Remux|IMAX-E|2160p
-
Merge Qualities
First we're going to merge a couple of qualities.

If you don't know how to merge qualities take a look at the following Guide:

=> How to Merge Quality

Merge the following Qualities together
Remux-2160p
WEBDL-2160p
WEBRip-2160p
Remux-1080p
WEBDL-1080p
WEBRip-1080p

and name it: WEB|Remux|2160p
-
Select the following qualities
The merged quality profile: WEB|Remux|2160p
-
Move selected quality to top
The order listed in the profile matters even if a quality is not checked. For example, if you have a 1080p version but want the SD version, Radarr will reject all SD results because 1080p is listed higher than SD, even though 1080p was not checked.

Qualities at the top of the list will appear first in manual searches.

Qualities higher in the list are more preferred, even if not checked.
Qualities within the same group are equal.
Only checked qualities are wanted.

This is why moving the selected quality to the top of the list is recommended.

Quality Profile Settings
Enable: Upgrades Allowed
Upgrade Until Quality: WEBDL|Remux|2160p
Minimum Custom Format Score: 3350 (1)
Upgrade Until Custom Format Score: 10000

(1) If you're limited to public indexers, lack access to top-tier indexers, or are searching for rarer content, you may want to lower the Minimum Custom Format Score to 10. (The minimum score of 3350 (WEB Tier 03 + DTS + HDR) ensures you don't receive any non-tiered releases.)

Always follow the data described in the guide.
If you have any questions or are unsure, don't hesitate to ask.
-
Custom Formats and scores
The following Custom Formats are required:
-
Audio
| Custom Format     | Score |
|-------------------|------:|
| TrueHD ATMOS      |  5000 |
| DTS X             |  4500 |
| ATMOS (undefined) |  3000 |
| DD+ ATMOS         |  3000 |
| TrueHD            |  2750 |
| DTS-HD MA         |  2500 |
| FLAC              |  2250 |
| PCM               |  2250 |
| DTS-HD HRA        |  2000 |
| DD+               |  1750 |
| DTS-ES            |  1500 |
| DTS               |  1250 |
| AAC               |  1000 |
| DD                |   750 |

HDR Formats Setup
HDR

All users with HDR-capable equipment should add the HDR custom format.

This is a catch-all custom format for all HDR-related formats, including those with HDR10 or HDR10+ fallback capabilities, such as DV HDR10 or DV HDR10+.

DV Boost

If you prefer Dolby Vision and have compatible equipment, add the DV Boost custom format. This custom format prioritizes releases containing Dolby Vision over standard HDR releases.

This custom format accepts DV Profile 5 and also upgrades from DV/HDR10/HDR10+ to DV HDR10 or DV HDR10+.

HDR10+ Boost

If you prefer HDR10+ releases and have compatible equipment, add the HDR10+ Boost custom format. This custom format prioritizes releases containing HDR10+ over standard HDR releases.

This custom format also boosts DV HDR10+ releases if you prefer them over DV HDR10.

Tip: If you prefer both Dolby Vision and HDR10+, add both boost custom formats!

DV (w/o HDR fallback)

If NOT every device accessing your media server supports Dolby Vision, add the DV (w/o HDR fallback) custom format to ensure maximum compatibility with your setup. This prevents playback issues on devices that don't fully support Dolby Vision.

This also applies to Dolby Vision releases without HDR10 fallback (Profile 5).


Recommendation: Add the DV (w/o HDR fallback) for maximum compatibility across all devices.

| Custom Format         |  Score |
|-----------------------|-------:|
| HDR                   |    500 |
| DV Boost              |   1000 |
| HDR10+ Boost          |    100 |
| DV (w/o HDR fallback) | -10000 |
 
-
HQ Release Groups
| Custom Format      | Score    |
|--------------------|----------|
| Remux Tier 01      | 1950     |
| Remux Tier 02      | 1900     |
| Remux Tier 03      | 1850     |
| WEB Tier 01        | 1700     |
| WEB Tier 02        | 1650     |
| WEB Tier 03        | 1600     |


:alert: "Scores marked with a ⚠️ sign are different from those used in the main public guide." :alert:
-
Miscellaneous (Required)
| Custom Format |   Score    |
|---------------|:----------:|
| Repack/Proper |     5      |
| Repack2       |     6      |
| Repack3       |     7      |
| x264          | ⚠ -10000 ⚠ |


Breakdown and Why

x264 has a score of -10000 because we only want the HDR/DV versions of the WEBDL-1080p.

:arrow: If you're only running 1 Radarr, you might want to remove the x264 CF so you will also get the HD release if there is no UHD version.

:Sirenevermelha: "Proper and Repacks"

We also suggest changing the Propers and Repacks settings in Radarr.

Media Management => File Management to Do Not Prefer and use the Repack/Proper Custom Format.

https://trash-guides.info/Radarr/images/cf-mm-propers-repacks-disable.png

This way, you ensure the Custom Formats preferences will be used and not ignored.
Image
-
Unwanted
| Custom Format         | Score  |
| --------------------- | :----: |
| BR-DISK               | -10000 |
| Generated Dynamic HDR | -10000 |
| LQ                    | -10000 |
| LQ (Release Title)    | -10000 |
| 3D                    | -10000 |
| Upscaled              | -10000 |
| Extras                | -10000 |
| Sing-Along Versions   | -10000 |
| AV1                   | -10000 |
-
Golden Rule
:Sirenevermelha: Please ensure you only score or enable one of them in your Quality Profile. :Sirenevermelha:

x265 (no HDR/DV): This blocks most 720/1080p (HD) releases that are encoded in x265, but it will allow 720/1080p x265 releases if they have HDR and/or DV.
x265 (HD): This blocks 720/1080p (HD) releases that are encoded in x265.

| Custom Format    | Score  |
|------------------|:------:|
| x265 (no HDR/DV) | -10000 |
| x265 (HD)        | -10000 |
-
Resolution
| Custom Format |  Score  |
|---------------|:-------:|
| 1080p         |   50    |
| 2160p         | ⚠ 151 ⚠ |


:alert: "Scores marked with a ⚠️ sign are different from those used in the main public guide." :alert:
-
TRaSHRole icon, Guides Contributor
OP
 — 19/06/2025 21:47
Streaming Services
| Custom Format | Score |
|---------------|:-----:|
| AMZN          |   0   |
| ATVP          |   0   |
| BCORE         |  15   |
| CRiT          |  20   |
| DSNP          |   0   |
| HBO           |   0   |
| HMAX          |   0   |
| Hulu          |   0   |
| iT            |   0   |
| MAX           |   0   |
| MA            |  20   |
| NF            |   0   |
| PMTP          |   0   |
| PCOK          |   0   |
| STAN          |   0   |


Breakdown and Why

These Custom Formats have a score of 0 because they are mainly used for the naming scheme. Other variables should decide if a certain release is preferred for movies. BCore, CRiT and MA` are the only ones with a positive score because they have better source material or a higher bitrate and quality than other streaming services.

The following Custom Formats are optional
-
Miscellaneous (Optional)
| Custom Format            | Score  |
| ------------------------ | :----: |
| Bad Dual Groups          | -10000 |
| Black and White Editions | -10000 |
| No-RlsGroup              | -10000 |
| Obfuscated               | -10000 |
| Retags                   | -10000 |
| Scene                    | -10000 |
-
SDR (Optional)
A group of custom formats that affect which SDR releases, if any, are grabbed in UHD-enabled profiles.
Please read the descriptions of the Collection of Custom Formats page for more details.

SDR: This will prevent grabbing UHD/4k releases without HDR Formats.
SDR (no WEBDL): This will prevent grabbing UHD/4k Remux and Blu-ray encode releases without HDR Formats. - i.e., SDR WEB releases will still be allowed since 4K SDR WEB releases can often look better than the 1080p version due to the improved bitrate.

| Custom Format     | Score  |
| ----------------- | ------ |
| SDR               | -10000 |
| SDR (no WEBDL) ⚠ | -10000 |


:Sirenevermelha: Please ensure you only score or enable one of them in your Quality Profile. :Sirenevermelha:

Movie Versions (Optional)
| Custom Format        | Score |
|----------------------|:-----:|
| Hybrid               |  100  |
| Remaster             |  25   |
| 4K Remaster          |  25   |
| Criterion Collection |  25   |
| Masters of Cinema    |  25   |
| Vinegar Syndrome     |  25   |
| Special Edition      |  125  |
| IMAX                 |  800  |
| IMAX Enhanced        |  800  |


IMAX: IMAX films are shot in tall aspect ratios, typically 1.9:1 or sometimes 1.43:1. Most IMAX film releases also have scenes shot at wider aspect ratios, and as a result, the aspect ratio will change throughout. Because they are shot on large-format cameras, there is often less film grain present, resulting in a clearer picture.
IMAX Enhanced exclusive expanded aspect ratio is 1:90:1, which offers up to 26% more picture for select sequences, meaning more of the action is visible on screen. IMAX Enhanced is a standard for digital releases. It features scenes shot on IMAX cameras and produced in HDR10 DV. IMAX Enhanced releases often have a higher bitrate than other WEB options and are encoded into various formats, including SDR conversions. Due to the higher bitrate and implied picture quality improvement, it is recommended that the IMAX Enhanced custom format be enabled on WEB profiles, especially for those seeking 'The IMAX Experience'—including fewer "black bars" or letterboxing.
:arrow: If you do not prefer IMAX Enhanced then do not add it, or use a score of 0. 
-
Quality Size
Settings => Quality

| Quality      | Min   | Max  |
|--------------|-------|------|
| WEBDL-1080p  | 12.5  | 2000 |
| WEBRip-1080p | 12.5  | 2000 |
| Remux-1080p  | 136.8 | 2000 |
| WEBDL-2160p  | 34.5  | 2000 |
| WEBRip-2160p | 34.5  | 2000 |
| Remux-2160p  | 187.4 | 2000 |


You don't see the Preferred score in the table above because we want max quality anyway, so we set it as high as possible.

The highest preferred quality you can manually enter is one less than the Maximum quality. If you use the slider, the preferred quality can be up to 5 lesser than the Maximum quality.

If you don't see a provision to enter the scores under the Quality settings, ensure you have enabled Show Advanced in Radarr.

