# Play Console forms: paths, fields, answers

Base: `https://play.google.com/console/u/<N>/developers/<devId>/app/<appId>/`.
New app: `/u/<N>/developers/<devId>/create-new-app`.

| Section | Path |
|---|---|
| Dashboard, task list | `app-dashboard` |
| Main store listing | `main-store-listing` |
| Category and contact details | `store-settings` |
| All declarations | `app-content/overview` (without `/overview` it redirects) |
| Privacy policy | `app-content/privacy-policy` |
| App access | `app-content/testing-credentials` |
| Ads | `app-content/ads-declaration` |
| Advertising ID | `app-content/ad-id-declaration` |
| Content rating | `app-content/content-rating-overview` → `content-rating-iarc-questionnaire` |
| Target audience | `app-content/target-audience-content` |
| Data safety | `app-content/data-privacy-security` |
| Government, financial, health apps | `app-content/government-apps`, `finance`, `health` |

## Creating the app

- **Fields:** app name (30), package name with a *Check availability* button, default language
  (dropdown), *App or game*, *Free or paid*.
- **At the bottom, two checkboxes:** the Developer Program Policies and US export laws.

## Listing and settings

- **Icon:** 512×512 PNG, up to 1 MB.
- **Feature graphic:** 1024×500. Without it the listing saves only as a draft.
- **Phone screenshots:** 2–8. The console says "16:9 or 9:16"; sides 320–3840 px, up to 8 MB. The
  long side may be at most twice the short one: raw 1280×2856 frames (2.2:1) are rejected, 1080×1920 passes.
- **7" and 10" tablets** carry an asterisk, but a full *Save* passes without them: the icon, the
  feature graphic and phone screenshots are enough.
- **Category and contacts:** the *Edit* dialogs on `store-settings`.

## Declarations

- **App access** ("Is any part of your app restricted?"): *No* if there is no sign-in and no paid content.
- **Ads:** *No* if no ad SDK is among the dependencies.
- **Advertising ID:** *No* only if the release manifest has no `com.google.android.gms.permission.AD_ID`.
- **Government apps:** *No*.
- **Financial features, health:** at the bottom of the list, the "My app doesn't …" checkbox, then *Next* → *Save*.
- **Target audience:** the console disables the under-13 groups itself when the rating is teen.
  Choosing only *18 and over* skips steps 2–4 and goes straight to the summary.
- **IARC:**
  - step 1: email, category (for an ordinary app, *All other app types*), the IARC terms checkbox;
  - *User-generated or internet content* (content not in the APK, such as product cards or feeds) = *Yes*;
  - questions about that outside content follow: violence, sex, language, age-restricted goods.
    Answer from the human's answers; never guess;
  - *Save* at the bottom keeps a draft of the questionnaire.
- **Data safety:**
  1. Overview.
  2. Does the app collect data, encryption in transit, accounts (*Users can't create an account* →
     sign-in with other accounts *No*), deletion requests (optional). Answer *Yes* only if there is a
     way to request deletion. *No, data is deleted within 90 days* only if everything, aggregates
     included, lives ≤ 90 days. Otherwise *No*.
  3. Data types. App events and search go under *App activity*; an install ID goes under
     *Device or other IDs*.
  4. For each type:
     - *Collected*; *Shared* only if the data reaches third parties (your own analytics on your own
       server is not sharing);
     - *Processed ephemerally?* is *No* if the data is stored;
     - *Required* if the user cannot turn collection off;
     - purpose: events and IDs are *Analytics*, a query used to serve results is *App functionality*.
  5. Preview: *Show all*, read it through JS `innerText` after the preview heading, and check it
     before *Save*.

## After filling in

- A new personal developer account needs a closed test before production: 12 testers for 14 days.
- Upload the AAB to internal testing (`droidship gplay upload <pkg> --aab F --track internal`).
  Send for review from *Publishing overview*, and only on the human's yes.
