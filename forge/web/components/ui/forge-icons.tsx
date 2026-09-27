import type { FC, SVGProps } from "react";
import { chart } from "@/lib/design-tokens";

// SVG fill/stroke attributes are not a CSS context, so `var(--token)` cannot be
// used here — every colour below comes from the shared chart/SVG palette.

export type IconProps = SVGProps<SVGSVGElement> & {
  size?: number | string;
  strokeWidth?: number | string;
};

// ─── 1. Brand Logo: Forge 4-blade Compass / Star ───────────────────────────
export const ForgeLogoIcon: FC<IconProps> = ({ size = 20, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    className={className}
    {...props}
  >
    <path
      d="M12 2L14.5 9.5L22 12L14.5 14.5L12 22L9.5 14.5L2 12L9.5 9.5L12 2Z"
      fill={chart.dangerBright}
    />
    <path
      d="M12 5.5L13.8 10.2L18.5 12L13.8 13.8L12 18.5L10.2 13.8L5.5 12L10.2 10.2L12 5.5Z"
      fill={chart.dangerSoft}
    />
    <circle cx="12" cy="12" r="2" fill={chart.onColor} />
  </svg>
);

// ─── 2. Overview: Dashboard Window Frame with Layout Panes ─────────────────
export const OverviewDashboardIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <rect x="3" y="3" width="18" height="18" rx="2.5" />
    <path d="M3 8.5H21" />
    <path d="M9.5 8.5V21" />
    <circle cx="6" cy="5.75" r="0.75" fill="currentColor" stroke="none" />
    <circle cx="8.5" cy="5.75" r="0.75" fill="currentColor" stroke="none" />
  </svg>
);

// ─── 3. Monitoring: Waveform Monitor on Grid ───────────────────────────────
export const MonitoringPulseIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <rect x="3" y="4" width="18" height="16" rx="2" strokeDasharray="1 3" opacity="0.35" />
    <path d="M3 13H7L9.5 6L14.5 18L17 13H21" />
  </svg>
);

// ─── 4. Health: Heart with ECG Rhythm Pulse ────────────────────────────────
export const HealthECGIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M19.5 12.572L12 20L4.5 12.572A5.2 5.2 0 0 1 12 5.5A5.2 5.2 0 0 1 19.5 12.572Z" opacity="0.45" />
    <path d="M3 13.5H7.5L9.5 9.5L13 17.5L15 13.5H21" />
  </svg>
);

// ─── 5. Activity: Pulse Electrocardiogram Squiggle ─────────────────────────
export const ActivityWaveIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M2 12H6L9 4L15 20L18 12H22" />
  </svg>
);

// ─── 6. Servers: Dual Rack Units with Indicator LED Dots ───────────────────
export const ServerRackIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <rect x="2" y="3" width="20" height="8" rx="2" />
    <rect x="2" y="13" width="20" height="8" rx="2" />
    <line x1="6" y1="7" x2="14" y2="7" />
    <line x1="6" y1="17" x2="14" y2="17" />
    <circle cx="18" cy="7" r="1" fill="currentColor" stroke="none" />
    <circle cx="18" cy="17" r="1" fill="currentColor" stroke="none" />
  </svg>
);

// ─── 7. Applications: 3D Isometric Wireframe Cube ──────────────────────────
export const ApplicationsCubeIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M12 2.5L20.5 7.5V16.5L12 21.5L3.5 16.5V7.5L12 2.5Z" />
    <path d="M12 12L20.5 7.5" />
    <path d="M12 12V21.5" />
    <path d="M12 12L3.5 7.5" />
  </svg>
);

// ─── 8. Databases: 3D Cylinder with Disk Segments ──────────────────────────
export const DatabaseCylinderIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <ellipse cx="12" cy="5" rx="8" ry="3" />
    <path d="M4 5V12C4 13.66 7.58 15 12 15C16.42 15 20 13.66 20 12V5" />
    <path d="M4 12V19C4 20.66 7.58 22 12 22C16.42 22 20 20.66 20 19V12" />
  </svg>
);

// ─── 9. Game Servers: Gamepad Controller with D-Pad & Buttons ──────────────
export const GamepadIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M6 11H10M8 9V13" />
    <circle cx="15.5" cy="11.5" r="0.8" fill="currentColor" stroke="none" />
    <circle cx="17.5" cy="9.5" r="0.8" fill="currentColor" stroke="none" />
    <rect x="2" y="6" width="20" height="12" rx="6" />
  </svg>
);

// ─── 10. App Store: Storefront Awning with Pillar Entrance ─────────────────
export const AppStoreIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M3 9L4.5 4H19.5L21 9V10C21 11.1 20.1 12 19 12C17.9 12 17 11.1 17 10C17 11.1 16.1 12 15 12C13.9 12 13 11.1 13 10C13 11.1 12.1 12 11 12C9.9 12 9 11.1 9 10C9 11.1 8.1 12 7 12C5.9 12 5 11.1 5 10C5 11.1 4.1 12 3 10V9Z" />
    <path d="M4 12V20H20V12" />
    <path d="M9 20V14H15V20" />
  </svg>
);

// ─── 11. Templates / Blueprints: Code Sheet with Folded Corner ─────────────
export const TemplateSheetIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M14 2H6C4.9 2 4 2.9 4 4V20C4 21.1 4.9 22 6 22H18C19.1 22 20 21.1 20 20V8L14 2Z" />
    <path d="M14 2V8H20" />
    <line x1="8" y1="13" x2="16" y2="13" />
    <line x1="8" y1="17" x2="13" y2="17" />
  </svg>
);

// ─── 12. Deployments: Sleek Launch Rocket at 45 Degrees ────────────────────
export const RocketLaunchIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M4.5 16.5C3 18 3 21 3 21C3 21 6 21 7.5 19.5L12 15L9 12L4.5 16.5Z" />
    <path d="M12 15L19.5 7.5C20.5 6.5 21 4.5 21 3C19.5 3 17.5 3.5 16.5 4.5L9 12" />
    <path d="M15 9L18 6" />
    <path d="M14 17L17 20" />
    <path d="M7 10L4 7" />
  </svg>
);

// ─── 13. Pipelines: Stage Nodes Connected by Flow Arteries ─────────────────
export const PipelineFlowIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <circle cx="5" cy="12" r="2.5" />
    <circle cx="12" cy="12" r="2.5" />
    <circle cx="19" cy="12" r="2.5" />
    <line x1="7.5" y1="12" x2="9.5" y2="12" />
    <line x1="14.5" y1="12" x2="16.5" y2="12" />
  </svg>
);

// ─── 14. Git Branch: Fork & Merge Source Control Tree ──────────────────────
export const GitBranchTreeIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <line x1="6" y1="3" x2="6" y2="21" />
    <circle cx="6" cy="6" r="2.5" />
    <circle cx="6" cy="18" r="2.5" />
    <path d="M6 9C6 11.5 8 13.5 10.5 13.5H15" />
    <circle cx="17.5" cy="13.5" r="2.5" />
  </svg>
);

// ─── 15. Compose: Stacked 3D Sheets / Layers ──────────────────────────────
export const ComposeSheetsIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <polygon points="12 2 2 7 12 12 22 7 12 2" />
    <polyline points="2 12 12 17 22 12" />
    <polyline points="2 17 12 22 22 17" />
  </svg>
);

// ─── 16. Nodes: Host Server / Processor with Pin Leads ─────────────────────
export const NodeHostIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <rect x="5" y="5" width="14" height="14" rx="2" />
    <circle cx="12" cy="12" r="3" />
    <line x1="9" y1="1" x2="9" y2="5" />
    <line x1="15" y1="1" x2="15" y2="5" />
    <line x1="9" y1="19" x2="9" y2="23" />
    <line x1="15" y1="19" x2="15" y2="23" />
    <line x1="1" y1="9" x2="5" y2="9" />
    <line x1="1" y1="15" x2="5" y2="15" />
    <line x1="19" y1="9" x2="23" y2="9" />
    <line x1="19" y1="15" x2="23" y2="15" />
  </svg>
);

// ─── 17. CPU KPI Card: Microchip SVG matching Screenshot ───────────────────
export const CpuKpiChipIcon: FC<IconProps> = ({ size = 18, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    className={className}
    {...props}
  >
    <rect x="4" y="4" width="16" height="16" rx="3" stroke={chart.blue} strokeWidth="1.8" />
    <rect x="8" y="8" width="8" height="8" rx="1.5" fill={chart.blue} fillOpacity="0.25" stroke={chart.blue} strokeWidth="1.5" />
    {/* Pins */}
    <line x1="9" y1="1" x2="9" y2="4" stroke={chart.blue} strokeWidth="1.5" strokeLinecap="round" />
    <line x1="15" y1="1" x2="15" y2="4" stroke={chart.blue} strokeWidth="1.5" strokeLinecap="round" />
    <line x1="9" y1="20" x2="9" y2="23" stroke={chart.blue} strokeWidth="1.5" strokeLinecap="round" />
    <line x1="15" y1="20" x2="15" y2="23" stroke={chart.blue} strokeWidth="1.5" strokeLinecap="round" />
    <line x1="1" y1="9" x2="4" y2="9" stroke={chart.blue} strokeWidth="1.5" strokeLinecap="round" />
    <line x1="1" y1="15" x2="4" y2="15" stroke={chart.blue} strokeWidth="1.5" strokeLinecap="round" />
    <line x1="20" y1="9" x2="23" y2="9" stroke={chart.blue} strokeWidth="1.5" strokeLinecap="round" />
    <line x1="20" y1="15" x2="23" y2="15" stroke={chart.blue} strokeWidth="1.5" strokeLinecap="round" />
  </svg>
);

// ─── 18. Memory KPI Card: Literal RAM Stick DIMM SVG matching Screenshot ───
export const MemoryRamStickIcon: FC<IconProps> = ({ size = 18, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    className={className}
    {...props}
  >
    {/* Stick body */}
    <rect x="2" y="7" width="20" height="10" rx="1.5" stroke={chart.violet} strokeWidth="1.8" />
    {/* 3 Memory chip dies */}
    <rect x="4.5" y="9.5" width="3.5" height="5" rx="0.5" fill={chart.violet} fillOpacity="0.4" />
    <rect x="10.25" y="9.5" width="3.5" height="5" rx="0.5" fill={chart.violet} fillOpacity="0.4" />
    <rect x="16" y="9.5" width="3.5" height="5" rx="0.5" fill={chart.violet} fillOpacity="0.4" />
    {/* Gold contact fingers along bottom */}
    <line x1="4" y1="17" x2="4" y2="19.5" stroke={chart.violet} strokeWidth="1.2" strokeLinecap="round" />
    <line x1="6.5" y1="17" x2="6.5" y2="19.5" stroke={chart.violet} strokeWidth="1.2" strokeLinecap="round" />
    <line x1="9" y1="17" x2="9" y2="19.5" stroke={chart.violet} strokeWidth="1.2" strokeLinecap="round" />
    <line x1="15" y1="17" x2="15" y2="19.5" stroke={chart.violet} strokeWidth="1.2" strokeLinecap="round" />
    <line x1="17.5" y1="17" x2="17.5" y2="19.5" stroke={chart.violet} strokeWidth="1.2" strokeLinecap="round" />
    <line x1="20" y1="17" x2="20" y2="19.5" stroke={chart.violet} strokeWidth="1.2" strokeLinecap="round" />
  </svg>
);

// ─── 19. Storage KPI Card: Stacked Disk Drive Platters matching Screenshot ───
export const StoragePlattersIcon: FC<IconProps> = ({ size = 18, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    className={className}
    {...props}
  >
    <ellipse cx="12" cy="6" rx="8" ry="3" stroke={chart.orange} strokeWidth="1.8" />
    <path d="M4 6V12C4 13.66 7.58 15 12 15C16.42 15 20 13.66 20 12V6" stroke={chart.orange} strokeWidth="1.8" />
    <path d="M4 12V18C4 19.66 7.58 21 12 21C16.42 21 20 19.66 20 18V12" stroke={chart.orange} strokeWidth="1.8" />
  </svg>
);

// ─── 20. Planet Icon for Bottom Project Switcher ───────────────────────────
export const PlanetDefaultIcon: FC<IconProps> = ({ size = 16, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    className={className}
    {...props}
  >
    <defs>
      <linearGradient id="planetGrad" x1="0" y1="0" x2="24" y2="24" gradientUnits="userSpaceOnUse">
        <stop stopColor={chart.success} />
        <stop offset="0.5" stopColor={chart.blue} />
        <stop offset="1" stopColor={chart.indigo} />
      </linearGradient>
    </defs>
    <circle cx="12" cy="12" r="7" fill="url(#planetGrad)" />
    <ellipse
      cx="12"
      cy="12"
      rx="11"
      ry="3.5"
      transform="rotate(-25 12 12)"
      stroke={chart.sky}
      strokeWidth="1.5"
      strokeDasharray="20 4 6 4"
    />
  </svg>
);

// ─── 21. System Health Green Operational Circular Badge ────────────────────
export const SystemHealthOperationalIcon: FC<IconProps> = ({ size = 28, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 32 32"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    className={className}
    {...props}
  >
    <circle cx="16" cy="16" r="14" stroke={chart.success} strokeWidth="2.5" strokeOpacity="0.4" />
    <circle cx="16" cy="16" r="10" fill={chart.success} />
    <path
      d="M12 16.5L14.5 19L20 13.5"
      stroke={chart.onColor}
      strokeWidth="2.5"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

// ─── 22. System Health Attention Circular Badge ────────────────────────────
export const SystemHealthAlertIcon: FC<IconProps> = ({ size = 28, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 32 32"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    className={className}
    {...props}
  >
    <circle cx="16" cy="16" r="14" stroke={chart.critical} strokeWidth="2.5" strokeOpacity="0.4" />
    <circle cx="16" cy="16" r="10" fill={chart.critical} />
    <path d="M16 11V17" stroke={chart.onColor} strokeWidth="2.2" strokeLinecap="round" />
    <circle cx="16" cy="20.5" r="1.2" fill={chart.onColor} />
  </svg>
);

// ─── 22a. System Health Unknown Circular Badge ─────────────────────────────
/**
 * The verdict is not in yet, or could not be read.
 *
 * Deliberately not the operational badge: a green tick before the sources
 * have answered is a claim the panel cannot support. Deliberately not the
 * alert badge either — nothing is known to be wrong, so an alarm would be
 * just as untrue in the other direction.
 */
export const SystemHealthUnknownIcon: FC<IconProps> = ({ size = 28, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 32 32"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    className={className}
    {...props}
  >
    <circle cx="16" cy="16" r="14" stroke={chart.unknown} strokeWidth="2.5" strokeOpacity="0.4" />
    <circle cx="16" cy="16" r="10" fill={chart.unknown} />
    <path d="M12 16H20" stroke={chart.onColor} strokeWidth="2.2" strokeLinecap="round" />
  </svg>
);

// ─── 23. Globe Grid for Regions / Domains ──────────────────────────────────
export const GlobeGridIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <circle cx="12" cy="12" r="9" />
    <line x1="3" y1="12" x2="21" y2="12" />
    <path d="M12 3C14.5 5.5 16 8.5 16 12C16 15.5 14.5 18.5 12 21C9.5 18.5 8 15.5 8 12C8 8.5 9.5 5.5 12 3Z" />
  </svg>
);

// ─── 24. Map Pin for Locations ─────────────────────────────────────────────
export const LocationPinIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M12 2C8.13 2 5 5.13 5 9C5 14.25 12 22 12 22C12 22 19 14.25 19 9C19 5.13 15.87 2 12 2Z" />
    <circle cx="12" cy="9" r="2.5" fill="currentColor" stroke="none" />
  </svg>
);

// ─── 25. Shipping Container for Docker / Runtime ───────────────────────────
export const ContainerShippingIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <rect x="2" y="5" width="20" height="14" rx="2" />
    <line x1="7" y1="5" x2="7" y2="19" />
    <line x1="12" y1="5" x2="12" y2="19" />
    <line x1="17" y1="5" x2="17" y2="19" />
  </svg>
);

// ─── 26. Network Plug Link for Endpoints ───────────────────────────────────
export const EndpointPlugIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M9 2V6M15 2V6" />
    <rect x="6" y="6" width="12" height="7" rx="1.5" />
    <path d="M12 13V22" />
    <path d="M9 18H15" />
  </svg>
);

// ─── 27. Router Gateway for Gateways ───────────────────────────────────────
export const GatewayRouterIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <rect x="2" y="13" width="20" height="8" rx="2" />
    <line x1="6" y1="13" x2="6" y2="4" />
    <line x1="18" y1="13" x2="18" y2="4" />
    <circle cx="6" cy="4" r="1" fill="currentColor" stroke="none" />
    <circle cx="18" cy="4" r="1" fill="currentColor" stroke="none" />
    <line x1="6" y1="17" x2="6.01" y2="17" strokeWidth="2.5" />
    <line x1="10" y1="17" x2="10.01" y2="17" strokeWidth="2.5" />
    <line x1="14" y1="17" x2="14.01" y2="17" strokeWidth="2.5" />
  </svg>
);

// ─── 28. Load Balancer: Balanced Traffic Split ─────────────────────────────
export const LoadBalancerSplitIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <line x1="4" y1="12" x2="10" y2="12" />
    <path d="M10 12L15 6H20" />
    <path d="M10 12L15 18H20" />
    <polyline points="18 4 20 6 18 8" />
    <polyline points="18 16 20 18 18 20" />
  </svg>
);

// ─── 29. Network Mesh Connected Nodes ──────────────────────────────────────
export const NetworkMeshNodesIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <circle cx="6" cy="6" r="2.5" />
    <circle cx="18" cy="6" r="2.5" />
    <circle cx="12" cy="18" r="2.5" />
    <line x1="8.5" y1="6" x2="15.5" y2="6" />
    <line x1="7.5" y1="8" x2="10.5" y2="16" />
    <line x1="16.5" y1="8" x2="13.5" y2="16" />
  </svg>
);

// ─── 30. Radio Beacon Tower for Subsystems ────────────────────────────────
export const BeaconRadioTowerIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M12 2L6 22H18L12 2Z" />
    <circle cx="12" cy="6" r="1.5" fill="currentColor" stroke="none" />
    <path d="M8 12H16" />
    <path d="M6.5 17H17.5" />
    <path d="M17 4C18.5 5.5 18.5 8.5 17 10" />
    <path d="M7 4C5.5 5.5 5.5 8.5 7 10" />
  </svg>
);

// ─── 31. Notification Bell with Badge ─────────────────────────────────────
export const NotificationBellIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <path d="M18 8A6 6 0 0 0 6 8C6 15 3 17 3 17H21S18 15 18 8" />
    <path d="M13.73 21A2 2 0 0 1 10.27 21" />
  </svg>
);

// ─── 32. Settings Cogwheel ────────────────────────────────────────────────
export const SettingsCogIcon: FC<IconProps> = ({ size = 16, strokeWidth = 1.75, className, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
    {...props}
  >
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15A1.65 1.65 0 0 0 20 16.5L20.8 17.8A2 2 0 0 1 19 20.8L17.5 20A1.65 1.65 0 0 0 15.5 20.5L15 22A2 2 0 0 1 12.8 22H11.2A2 2 0 0 1 9 20.5L8.5 19A1.65 1.65 0 0 0 6.5 18.5L5 19.3A2 2 0 0 1 2.2 16.5L3 15A1.65 1.65 0 0 0 2.5 13.5L1 12.8A2 2 0 0 1 1 11.2L2.5 10.5A1.65 1.65 0 0 0 3 9L2.2 7.5A2 2 0 0 1 5 4.7L6.5 5.5A1.65 1.65 0 0 0 8.5 5L9 3.5A2 2 0 0 1 11.2 2H12.8A2 2 0 0 1 15 3.5L15.5 5A1.65 1.65 0 0 0 17.5 5.5L19 4.7A2 2 0 0 1 21.8 7.5L21 9A1.65 1.65 0 0 0 21.5 10.5L23 11.2A2 2 0 0 1 23 12.8L21.5 13.5A1.65 1.65 0 0 0 21 15Z" />
  </svg>
);
