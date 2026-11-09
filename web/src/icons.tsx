// Inline SVG icons (no icon font, nothing fetched). Decorative: always aria-hidden.
const paths = {
  inbox: "M2.5 9.5h3l1 2h3l1-2h3M2.5 9.5 4 3.5h8l1.5 6v3h-11z",
  new: "M8 2.5v3M8 10.5v3M2.5 8h3M10.5 8h3M4.2 4.2l1.6 1.6M10.2 10.2l1.6 1.6M4.2 11.8l1.6-1.6M10.2 5.8l1.6-1.6",
  hidden: "M2 8s2.2-4 6-4 6 4 6 4-2.2 4-6 4-6-4-6-4zM2.5 2.5l11 11",
  preset: "M3 4h10M3 8h10M3 12h6",
  rail: "M2.5 3.5h11v9h-11zM6 3.5v9",
  chevron: "M6 4l4 4-4 4",
  close: "M4 4l8 8M12 4l-8 8",
  filter: "M2.5 4h11M4.5 8h7M6.5 12h3",
  sun: "M8 5.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5zM8 1.5v1.5M8 13v1.5M1.5 8H3M13 8h1.5M3.4 3.4l1 1M11.6 11.6l1 1M3.4 12.6l1-1M11.6 4.4l1-1",
  moon: "M12.5 9.5A5 5 0 0 1 6.5 3.5a5 5 0 1 0 6 6z",
  system: "M2.5 3.5h11v7h-11zM6 13.5h4M8 10.5v3",
  back: "M10 3.5 5.5 8l4.5 4.5",
  search: "M7 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8zM10 10l3.5 3.5",
};

export type IconName = keyof typeof paths;

export function Icon({ name }: { name: IconName }) {
  return (
    <svg className="icon" viewBox="0 0 16 16" aria-hidden="true" focusable="false">
      <path d={paths[name]} />
    </svg>
  );
}
