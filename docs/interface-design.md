# Desktop interface

The Wails interface follows Apple's current macOS design guidance:

- Keep translucent material in navigation; use clear surfaces for settings and data.
- Use system typography, familiar control sizes, restrained blue accents, and grouped settings.
- Carry the Finder-style sidebar through flat content surfaces and rounded controls. Group content with compact spacing instead of nested cards or divider lines; preserve the existing appearance colors.
- Keep Activity logs in one shared surface and CLI status, enable, and path controls compact. Use shared control heights, corner radii, and content gutters across screens.
- Keep content independently scrollable and the primary navigation available.
- Follow system appearance and respect reduced motion, reduced transparency, and increased contrast.
- Support keyboard navigation, visible focus, labelled controls, and text alongside status colors.

The interface uses CSS and Lucide icons; the macOS window uses Wails' integrated title bar and native translucency. It approximates the current macOS visual language rather than implementing AppKit's Liquid Glass controls.

## References

- [Apple: Build an AppKit app with the new design](https://developer.apple.com/videos/play/wwdc2025/310/)
- [Apple: Adopting Liquid Glass](https://developer.apple.com/documentation/technologyoverviews/adopting-liquid-glass)
- [Apple: Designing for macOS](https://developer.apple.com/design/human-interface-guidelines/designing-for-macos)

## Preview

Run `npm --prefix frontend run dev` and open `http://127.0.0.1:5173/?preview`. Sample accounts and changes live only in memory. Production builds omit the sample backend. Run Wails development mode to review the actual native window and saved services.

Shortcuts: ⌘1–3 switches screens, ⌘F searches, and ⌘N adds a service. Search supports service names, IDs, and origins; use arrow keys to move through the results and Return to select.
