SQP-1 WEB (1080p) - Streaming Optimized

Why choose this quality profile
You run a second "Streaming Optimized" instance of Radarr and want both versions, or you just want streaming-optimized 1080p overall.
You want fewer upgrades than with SQP-1 because the WEBDL is often watched before the BHDStudio Blu-ray is released.
You desire good compatibility across various playback devices without compromising release quality.
You want little to no transcoding for low-powered devices or remote streaming.
Smaller file sizes and bitrates ( the same quality as you would get with official streaming services).
Easier to obtain even without access to top-tier trackers or indexers.
You prefer Dolby Digital Plus (Atmos) audio over Dolby Digital, as WEB-DLs commonly come with Dolby Digital Plus (Atmos) audio.
You prefer IMAX-Enhanced.
You prefer included embedded subtitles.
More cross-seed hits, as WEBDL is more widespread over the trackers.

BHDStudio release HQ 1080p Encodes with the following features:

Streaming optimized (Optimized for PLEX, emby, Jellyfin, and other streaming platforms)
AC3 Audio (Downmixed Lossless audio track to Dolby Digital 5.1 for optimal compatibility)
Small sizes
Good quality

If you don't have access to the top tier indexers, you won't have access to all BHDStudio releases. WEB releases, however, have much better general availability.

hallowed release HQ 1080p encodes with the following features:

Highly streamable releases with some more modern optimizations:
Dolby Digital Plus multichannel or AAC mono/stereo audio (converted from lossless audio track)
Included English SRT subtitles, as well as a selection of PGS subtitles in English and other languages
Dual audio for foreign films
Smaller sizes
Good quality

hallowed's releases have very good general availability.

Workflow Logic
Depending what's released first and available the following Workflow Logic will be used:

When a 1080p WEBDL is released it will be downloaded.
When a 1080p BHDStudio/hallowed is released it won't be grabbed as an upgrade.
If no 1080p WEBDL can be found, a 1080p BHDStudio or hallowed release will be preferred (where the hallowed will be preferred over BHDStudio).
If no 1080p WEBDL or 1080p BHDStudio/hallowed can be found, a 1080p Bluray encode will be grabbed, although these can be less - or not at all - streaming optimized.
Releases with 2.0 audio will be scored lower, so a WEB Tier 01 could be trumped by a BHDStudio or any other HD Bluray Tier 03 that has, for example, 5.1 audio.

Possible Variables
Prefer 1080p WEBDL with IMAX-E.
 
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

Create a new profile and name it: SQP-1 WEB (1080p) 
-
Merge Qualities
First we're going to merge a couple of qualities.

If you don't know how to merge qualities take a look at the following Guide: https://trash-guides.info/Radarr/Tips/Merge-quality/

Merge the following Qualities together
Bluray-1080p
WEBDL-1080p
WEBRip-1080p

and name it: Bluray|WEB-1080p 
-
Select the following qualities
The merged quality profile: Bluray|WEB-1080p
 
-
Move selected quality to top
The order listed in the profile matters even if a quality is not checked. For example, if you have a 1080p version but want the SD version, Radarr will reject all SD results because 1080p is listed higher than SD, even though 1080p was not checked.

Qualities at the top of the list will appear first in manual searches.

Qualities higher in the list are more preferred, even if not checked.
Qualities within the same group are equal.
Only checked qualities are wanted.

This is why moving the selected quality to the top of the list is recommended.

Source: Wiki Servarr

Quality Profile Settings
Enable: Upgrades Allowed
Upgrade Until Quality: Bluray|WEB-1080p
Minimum Custom Format Score: 1000 (1)
Upgrade Until Custom Format Score: 10000

(1) If you're limited to public indexers, lack access to top-tier indexers, or are searching for rarer content, you may want to lower the Minimum Custom Format Score to 10. 

Always follow the data described in the guide.
If you have any questions or are unsure, don't hesitate to ask.
 
-
Custom Formats and scores
The following Custom Formats are required:
-
Audio
| Custom Format     | Score       |
|-------------------|-------------|
| TrueHD ATMOS      | ⚠ -10000 ⚠ |
| DTS X             | ⚠ -10000 ⚠ |
| ATMOS (undefined) | ⚠ 0 ⚠      |
| DD+ ATMOS         | ⚠ 135 ⚠    |
| TrueHD            | ⚠ -10000 ⚠ |
| DTS-HD MA         | ⚠ -10000 ⚠ |
| FLAC              | ⚠ 0 ⚠      |
| PCM               | ⚠ 0 ⚠      |
| DTS-HD HRA        | ⚠ -10000 ⚠ |
| DD+               | ⚠ 125 ⚠    |
| DTS-ES            | ⚠ 0 ⚠      |
| DTS               | ⚠ 0 ⚠      |
| AAC               | ⚠ 0 ⚠      |
| DD                | ⚠ 115 ⚠    |
| 2.0 Stereo        | ⚠ -175 ⚠   |


:alert: "Scores marked with a ⚠️ sign are different from those used in the main public guide." :alert:

The CF with 0 you can choose to add with a score of 0 or just don't add them. The reason why we score them this low is to prevent transcoding as much as possible.
The reason why DTS and DTS-ES have a score of 0 is to ensure you don't limit yourself too much.

HQ Release Groups
| Custom Format     |    Score   |
|-------------------|:----------:|
| BHDStudio         |     550    |
| hallowed          |     600    |
| WEB Tier 01       |    1700    |
| WEB Tier 02       |    1650    |
| WEB Tier 03       |    1600    |
| HD Bluray Tier 01 | ⚠ 1100 ⚠ |
| HD Bluray Tier 02 | ⚠ 1050 ⚠ |
| HD Bluray Tier 03 | ⚠ 1000 ⚠ |


:alert: "Scores marked with a ⚠️ sign are different from those used in the main public guide." :alert:
-
Miscellaneous (Required)
| Custom Format | Score |
|---------------|:-----:|
| Repack/Proper |   5   |
| Repack2       |   6   |
| Repack3       |   7   |


:Sirenevermelha: "Proper and Repacks"

We also suggest changing the Propers and Repacks settings in Radarr.

Media Management => File Management to Do Not Prefer and use the Repack/Proper Custom Format.

https://trash-guides.info/Radarr/images/cf-mm-propers-repacks-disable.png

This way, you ensure the Custom Formats preferences will be used and not ignored.

Unwanted
| Custom Format         |  Score |
|-----------------------|:------:|
| BR-DISK               | -10000 |
| Generated Dynamic HDR | -10000 |
| LQ                    | -10000 |
| LQ (Release Title)    | -10000 |
| x265 (HD)             | -10000 |
| 3D                    | -10000 |
| Extras                | -10000 |
| Sing-Along Versions   | -10000 |
| 10bit                 | -10000 |
| AV1                   | -10000 |
-
Resolution
| Custom Format | Score |
|---------------|:-----:|
| 1080p         |   50  |
| 720p          |   5   |
-
Streaming Services
| Custom Format |     Score    |
|---------------|:------------:|
| AMZN          |       0      |
| ATVP          |       0      |
| BCORE         | ⚠ -10000 ⚠  |
| CRiT          |      20      |
| DSNP          |       0      |
| HBO           |       0      |
| HMAX          |       0      |
| Hulu          |       0      |
| iT            |       0      |
| MAX           |       0      |
| MA            |      20      |
| NF            |       0      |
| PMTP          |       0      |
| PCOK          |       0      |
| STAN          |       0      |

:alert: "Scores marked with a ⚠️ sign are different from those used in the main public guide." :alert:

    ---

Breakdown and Why

These Custom Formats have a score of 0 because they are mainly used for the naming scheme. Other variables should decide if a certain release is preferred for movies.
CRiT and MA are the only ones with a positive score because of their better source material or higher bitrate and quality compared to other streaming services. BCore has a negative score as these releases have a very high bitrate, which can cause transcoding.
-
The following Custom Formats are optional
-
Miscellaneous (Optional)
| Custom Format            |  Score |
|--------------------------|:------:|
| Bad Dual Groups          | -10000 |
| Black and White Editions | -10000 |
| EVO (no WEBDL)           | -10000 |
| No-RlsGroup              | -10000 |
| Obfuscated               | -10000 |
| Retags                   | -10000 |
| Scene                    | -10000 |
-
Movie Versions (Optional)

:alert: "Scores marked with a ⚠️ sign are different from those used in the main public guide." :alert:

    ---

Breakdown and Why

These Custom Formats have a score of 0 because they are mainly used for the naming scheme. Other variables should decide if a certain release is preferred for movies.
CRiT and MA are the only ones with a positive score because of their better source material or higher bitrate and quality compared to other streaming services. BCore has a negative score as these releases have a very high bitrate, which can cause transcoding.
-
The following Custom Formats are optional
-
Miscellaneous (Optional)
| Custom Format            |  Score |
|--------------------------|:------:|
| Bad Dual Groups          | -10000 |
| Black and White Editions | -10000 |
| EVO (no WEBDL)           | -10000 |
| No-RlsGroup              | -10000 |
| Obfuscated               | -10000 |
| Retags                   | -10000 |
| Scene                    | -10000 |
-
Movie Versions (Optional)


If you prefer 1080p WEBDL with IMAX-E then add IMAX Enhanced with the default scores.
The reason why we don't add IMAX is because BHDStudio didn't add IMAX to their filename before 2023-07-27.

:alert: "Adding IMAX/IMAX Enhanced will often replace the BHDStudio release." :alert:
-
Quality Size
Settings => Quality

| Quality      | Min  | Max  |
|--------------|------|------|
| WEBDL-720p   | 12.5 | 85.7 |
| WEBRip-720p  | 12.5 | 85.7 |
| WEBDL-1080p  | 12.5 | 102  |
| WEBRip-1080p | 12.5 | 102  |
| Bluray-720p  | 25.2 | 102  |
| Bluray-1080p | 33.8 | 154  |


You don't see the Preferred score in the table above because we want max quality anyway, so we set it as high as possible.

The highest preferred quality you can manually enter is one less than the Maximum quality. If you use the slider, the preferred quality can be up to 5 lesser than the Maximum quality.

If you don't see a provision to enter the scores under the Quality settings, ensure you have enabled Show Advanced in Radarr

