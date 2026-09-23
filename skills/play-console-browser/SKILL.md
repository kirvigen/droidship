---
name: play-console-browser
description: Use when asked to create an app in Google Play Console or fill anything in it through a browser — "create the app in Google Play", "fill in the store listing", upload the icon, screenshots or feature graphic, "App content", Data safety, IARC content rating, target audience, ads and policy declarations, play.google.com/console. Covers what no Play API can do; releases and reviews go through droidship.
---

# Google Play Console in the browser

## First

- **No API can create an app.** Creating an app and everything under "App content" exist only in
  the web console. Drive a browser that holds the human's signed-in session (for example Claude in Chrome).
- Load the browser tools at once: tabs, navigate, computer, find, read_page, javascript, file upload.
- `droidship gplay …` answers 403 until the service account is added to the app under
  *Users and permissions*. Granting it changes permissions, so ask the human first.
- Tick consent boxes, send declarations and press *Send for review* only after the human's explicit
  yes. By default, only save.

## Ask everything in one batch, before the browser

- Which developer account. One login can hold several (`/u/N/`); let the human switch, or ask for
  the console URL.
- App name, applicationId, free or paid (a free app cannot become paid after publishing), category,
  contact email, privacy policy URL.
- Consent to the checkboxes: the Developer Program Policies, US export laws, the IARC terms.
- If the app shows content from outside the APK (catalogues, feeds): can it contain violence, sex,
  age-restricted goods, and is there a filter. IARC asks.
- How long data is kept, and whether users can request deletion. Data safety asks.

## Check the repository

1. Listing texts: find them in the repo or ask. Limits: title 30, short description 80, full 4000.
2. **Check the texts and screenshots against the code.** Everything promised (features, number of
   sources) must be in the app. A screenshot promises as much as the text does.
3. For Data safety:
   - analytics SDKs and the fields they send;
   - how long data is kept;
   - TLS on the backend and on the analytics server;
   - ad SDKs among the dependencies;
   - `AD_ID` in the merged release manifest (`build/intermediates/merged_manifest*/release`;
     build a release first if the directory is missing);
   - whether location is derived from the IP address.
4. The privacy policy loads with `curl` and does not contradict the SDKs. If it says "no analytics"
   and there is an analytics SDK, fix that before Data safety.

## Browser pitfalls

| Symptom | Fix |
|---|---|
| `Script injection timed out` on screenshot, find, or a click by ref | A few seconds after load the page is "busy". Clicks by coordinates still work. Reopen the URL and act at once, or wait 5–10 s and retry. If that fails, open another console page (`app-dashboard`) and come straight back |
| Screenshot comes back as a 2×2 grid, clicks miss | A second tab is open. Work in one tab, close the rest |
| A click by ref from `find` only scrolls | Click by coordinates from a fresh screenshot, or click via JS (below) |
| A click did nothing, the layout moved | Take a new screenshot after every click. The first click in a freshly opened dialog is often lost; repeat it |
| Typing ate spaces and punctuation | Insert with `insertText` (below) and compare `value.length` with the source |
| Long dropdown | Scroll with the wheel inside the list and click by coordinates. `scroll_to` on an item breaks the layout |
| A path redirects to the app list | Go through `app-content/overview`; it links every declaration |
| `input:checked` is empty on Material controls | Check visually, or on the preview step |

```js
// Text into a textarea. Do not assign .value: Material will not notice the change.
const ta = [...document.querySelectorAll('textarea')].find(t => t.value.includes(MARKER));
ta.focus(); ta.select();   // or ta.setSelectionRange(start, end) for a fragment
document.execCommand('insertText', false, TEXT); ta.value.length;

// Buttons a click by ref does not reach. Match the label in the console's language.
document.querySelector(`button[aria-label^="${LABEL}"]`).click();
[...document.querySelectorAll('button')].filter(b => b.textContent.trim().startsWith(LABEL)).forEach(b => b.click());
[...document.querySelectorAll('button')].filter(b => b.textContent.trim().endsWith(ADD_LABEL)).at(-1).click();
```

## Graphics and saving

- *Add assets* opens the media library on the right. `find` locates its `input type=file`; upload
  through it (browser tools often cap one upload at 10 MB). Uploaded files come pre-selected. The
  *Add* button at the bottom hides under the save bar: press it with a JS click (the last line above).
- Order in a slot follows the media library, not file names. Reorder with a drag onto the target
  tile and check with zoom. Hovering a tile shows delete (left) and move (right).
- *Save as draft* saves without the required graphics. A full *Save* needs the feature graphic.
  A toast is not proof: reload and check.

Console paths, form fields and declaration answers: `forms.md`.
