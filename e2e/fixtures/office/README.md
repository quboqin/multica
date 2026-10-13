# Office preview fixtures

Synthetic documents containing a heading, a table (Word), two slides
(PowerPoint), and Chinese text. Generated with python-docx and python-pptx;
legacy `.doc` / `.ppt` copies were exported with LibreOffice 26.8 in
Gotenberg 8.37.0. They contain no user data, macros, or external resources.

`e2e/office-preview-layout.spec.ts` uploads all four files through the API and
checks their authenticated PDF response and shared viewer. Run against a managed
test environment with `OFFICE_PREVIEW_URL` pointing to a private Gotenberg
instance configured as in `docker-compose.selfhost.yml`.
