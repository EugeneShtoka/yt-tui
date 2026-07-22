# Plan

1. [ ] Video thumbnail is not shown in the video detail overlay.
2. [ ] Open chapters/links works only from video detail overlay.
3. [ ] Playlists are not saved locally - they should be saved in the database, and when opened - showed immediately, and in the background, loaded from yt, and if different - update both UI and DB.
4. [ ] Add sort by tags to Channels tab.
5. [ ] Keybinding hints are not shown in status bar upon open, but do showing up after switching tabs back and force.
6. [ ] Most of tabs have at least one more row in table to show the content, but despite this it remains blank. But this is especially visible in tags tab - there about 5 lines of space are left blank at the bottom.
7. [ ] Complete the daemon transition and verify it is working in remote mode connecting via tui. This will also require separation of configs to tui and server parts.
8. [ ] Add multi-user support. For server, it is quite obvious, but need to brainstorm how to combine it with local mode.
