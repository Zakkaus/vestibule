import activity from "./lucide/activity.svg?raw";
import arrowRight from "./lucide/arrow-right.svg?raw";
import bookOpen from "./lucide/book-open.svg?raw";
import chartNoAxesCombined from "./lucide/chart-no-axes-combined.svg?raw";
import chevronDown from "./lucide/chevron-down.svg?raw";
import chevronUp from "./lucide/chevron-up.svg?raw";
import circleAlert from "./lucide/circle-alert.svg?raw";
import circleCheck from "./lucide/circle-check.svg?raw";
import circleHelp from "./lucide/circle-help.svg?raw";
import circleMinus from "./lucide/circle-minus.svg?raw";
import clipboardList from "./lucide/clipboard-list.svg?raw";
import inbox from "./lucide/inbox.svg?raw";
import info from "./lucide/info.svg?raw";
import languages from "./lucide/languages.svg?raw";
import layoutDashboard from "./lucide/layout-dashboard.svg?raw";
import listX from "./lucide/list-x.svg?raw";
import loaderCircle from "./lucide/loader-circle.svg?raw";
import messagesSquare from "./lucide/messages-square.svg?raw";
import monitor from "./lucide/monitor.svg?raw";
import moon from "./lucide/moon.svg?raw";
import pencil from "./lucide/pencil.svg?raw";
import plus from "./lucide/plus.svg?raw";
import refreshCw from "./lucide/refresh-cw.svg?raw";
import rotateCcw from "./lucide/rotate-ccw.svg?raw";
import rss from "./lucide/rss.svg?raw";
import save from "./lucide/save.svg?raw";
import settings from "./lucide/settings.svg?raw";
import shieldAlert from "./lucide/shield-alert.svg?raw";
import shieldCheck from "./lucide/shield-check.svg?raw";
import shieldOff from "./lucide/shield-off.svg?raw";
import slidersHorizontal from "./lucide/sliders-horizontal.svg?raw";
import sun from "./lucide/sun.svg?raw";
import trash2 from "./lucide/trash-2.svg?raw";
import undo2 from "./lucide/undo-2.svg?raw";
import unlock from "./lucide/unlock.svg?raw";
import usersRound from "./lucide/users-round.svg?raw";
import x from "./lucide/x.svg?raw";

const sources = {
  activity,
  arrowRight,
  bookOpen,
  chartNoAxesCombined,
  chevronDown,
  chevronUp,
  circleAlert,
  circleCheck,
  circleHelp,
  circleMinus,
  clipboardList,
  inbox,
  info,
  languages,
  layoutDashboard,
  listX,
  loaderCircle,
  messagesSquare,
  monitor,
  moon,
  pencil,
  plus,
  refreshCw,
  rotateCcw,
  rss,
  save,
  settings,
  shieldAlert,
  shieldCheck,
  shieldOff,
  slidersHorizontal,
  sun,
  trash2,
  undo2,
  unlock,
  usersRound,
  x
} as const;

export type IconName = keyof typeof sources;

type IconProps = Readonly<{
  name: IconName;
}>;

export function Icon({ name }: IconProps) {
  return (
    <span
      aria-hidden="true"
      data-icon
      data-icon-name={name}
      dangerouslySetInnerHTML={{ __html: sources[name] }}
    />
  );
}
