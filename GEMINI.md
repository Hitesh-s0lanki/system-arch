# Project Rules

## Frontend Folder Structure Convention

### App Router — Component Placement

This project uses Next.js App Router. Follow these rules **always** when creating or placing components:

#### Page-Level Components
Components that belong to a specific page **must** be colocated in a `_components` folder nested under that page's folder:

```
src/app/
└── (group)/
    └── <page-name>/
        ├── _components/     ← components used ONLY by this page
        │   ├── HeroSection.tsx
        │   └── FeatureCard.tsx
        └── page.tsx
```

> The `_components` folder uses the underscore prefix so Next.js treats it as a private folder and never routes it.

**Examples:**
- `src/app/(public)/dashboard/page.tsx` → its components go in `src/app/(public)/dashboard/_components/`
- `src/app/(auth)/login/page.tsx` → its components go in `src/app/(auth)/login/_components/`
- `src/app/(public)/page.tsx` (index route) → its components go in `src/app/(public)/_components/`

#### Shared / Common Components
Components **reused across multiple pages** must be placed in the top-level components directory:

```
src/
└── components/
    ├── ui/          ← shadcn/radix primitives
    └── <shared>.tsx ← other shared components
```

#### Summary Table

| Component Type | Location |
|---|---|
| Used by one specific page | `src/app/<route>/_components/` |
| Shared across multiple pages | `src/components/` |
| UI primitives (shadcn/ui) | `src/components/ui/` |

---

Always apply this structure when generating, moving, or refactoring components.

