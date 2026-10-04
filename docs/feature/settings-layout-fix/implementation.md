# Settings layout repair

F0/F1: Clean main719accb. User requests readable settings typography and non-pinned save row. P10 iteration, existing settings API/UI contract sufficient; no API changes. Implementation allowed. Acceptance: help text never overlaps labels; Git settings occupy full row; save remains normal document flow; desktop/mobile usable. Scope settings UI only.

F2: Existing absolute-positioned small elements incorrectly treat long help text as credential badges; nested Git fieldset occupies one grid column; loose paragraph inherits large type. Use normal-flow labels/help, full-width grouped cards and compact section headings. Put diagram toggle/description together. Save row after content, explicit unchanged/dirty status, disabled save while unchanged.

F3: Adjust settings.tsx/git-audit-settings.tsx scoped classes and workspace CSS. Build/typecheck, browser desktop/mobile fixture, preserve config/database while deploying main1234. Rollback previous binary; no migration.

F4/F5: Fixed long help absolute positioning, full-width Git group, scoped readable descriptions, unified diagram option card and normal-flow save row. Unchanged settings disable submit. TypeScript/Vite production build passed. Browser desktop verified help/save position static and full content-width Git card; mobile390px verified scrollWidth=clientWidth390 and single-column controls. Fixture screenshots /tmp/aimangebot-settings-fixed.png and /tmp/aimangebot-settings-mobile.png. No production settings modified.

F6: Scoped presentation change, existing save/validation/API retained; no backend migration. Generated embedded assets rebuilt. Ready for main/local1234 update.

F7/F8: Pushed main b168f5e, deployed rebuilt native binary on localhost1234 and verified new embedded asset. Private backup made; config bytes preserved; no active audits interrupted. Fixture server stopped. Presentation-only repair complete.
