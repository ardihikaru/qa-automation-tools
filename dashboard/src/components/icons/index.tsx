import { SVGProps } from 'react';

// Original icons, drawn on a 24x24 grid, 1.75 stroke, currentColor — to sit
// next to lucide-react icons.
type IconProps = SVGProps<SVGSVGElement>;

const base: IconProps = {
  viewBox: '0 0 24 24',
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.75,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
  'aria-hidden': true,
};

// App mark: a check inside a dashed scan ring (QA + automation).
export function LogoMark(props: IconProps) {
  return (
    <svg {...base} {...props}>
      <circle cx="12" cy="12" r="9.25" strokeDasharray="3 3" />
      <path d="M7.5 12.5l3 3 6-7" />
    </svg>
  );
}

// Products: a stacked box with a tag notch.
export function ProductsIcon(props: IconProps) {
  return (
    <svg {...base} {...props}>
      <path d="M12 3l8 4.5v9L12 21l-8-4.5v-9L12 3z" />
      <path d="M4 7.5l8 4.5 8-4.5" />
      <path d="M12 12v9" />
    </svg>
  );
}

// Settings / architecture: three linked nodes.
export function ArchitectureIcon(props: IconProps) {
  return (
    <svg {...base} {...props}>
      <rect x="9" y="3" width="6" height="5" rx="1.25" />
      <rect x="3" y="16" width="6" height="5" rx="1.25" />
      <rect x="15" y="16" width="6" height="5" rx="1.25" />
      <path d="M12 8v4M6 16v-4h12v4" />
    </svg>
  );
}

// Repository verification: a branch with a check.
export function RepoCheckIcon(props: IconProps) {
  return (
    <svg {...base} {...props}>
      <circle cx="6" cy="5.5" r="2" />
      <circle cx="6" cy="18.5" r="2" />
      <path d="M6 7.5v9" />
      <path d="M12 14.5l2.5 2.5 5-5.5" />
      <path d="M6 12c0-2 1.5-3 3.5-3H13" />
    </svg>
  );
}
