### Code Generation & Fixes

1. **Go:** Write strictly idiomatic Go code following standard formatting and best practices. Use Go html/template best practices.

2. **CSS:** You are generating code for a stack utilizing **TailwindCSS v4**, **Go `html/template`**, and a custom global CSS stylesheet. Adhere strictly to the following architectural rules:
    - **Hybrid Styling Philosophy:** Keep the use of Tailwind utility classes to an absolute minimum on individual HTML elements. Rely heavily on the base style definitions established in the global `app.css` file for elements like headings (`h1`-`h6`), paragraphs, and tables.
    - **Utility Class Restrictions:** Use Tailwind utility classes strictly for:
        * **Layout & Positioning:** `flex`, `grid`, `gap`, `justify-`, `items-`.
        * **Container Queries:** `@container`, `@md:flex`.
        * **Spacing:** `p-`, `m-` (always using logical properties like `ps-` or `ms-`).
        * **State Overrides:** Specific dynamic states handled by Go template logic.
    - **Semantic Color System:** The design system implements a PrimeVue-style palette with semantic colors (`primary`, `secondary`, `info`, `warn`, `danger`) and dynamic surface colors (`surface-0` through `surface-950`).
    - **Automatic Dark Mode:** Do not use Tailwind's `dark:` variant classes in the HTML. The CSS architecture handles dark mode automatically by reassigning the `--color-surface-*` variables based on system preference. Apply standard surface classes (e.g., `bg-surface-0`, `text-surface-900`) and rely on the CSS layer to invert them.
    - **Go Template Componentization:** Treat Go `{{ define "name" }}` blocks as isolated UI components. Pass data via template pipelines and manage UI states using Go conditional logic (e.g., `{{ if .HasError }}text-danger{{ end }}`).
    - **Logical Properties:** Exclusively use Tailwind logical property utilities (e.g., `ms-4`, `pe-6`, `text-start`) instead of physical directional properties (e.g., `ml-4`, `pr-6`, `text-left`) to guarantee LTR/RTL layout support.

3. **HTMX**: This tech stack is using HTMX v2.0.10. Follow HTMX and Hypermedia Systems best practices.

