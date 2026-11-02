export type CourierAction = 'run' | 'pause' | 'store' | 'notify' | 'pull' | 'seal' | 'persist' | 'permission' | 'deny' | 'complete'

export function Courier({ action = 'run', label }: { action?: CourierAction; label: string }) {
  return (
    <svg className={`courier courier-${action}`} viewBox="0 0 240 250" role="img" aria-label={label}>
      <g className="courier-tail">
        <ellipse cx="70" cy="114" rx="70" ry="94" fill="#B86F3C" />
        <ellipse cx="68" cy="110" rx="44" ry="66" fill="#F5F6F7" />
        <path d="M84 132h82v90H84z" fill="#F5F6F7" />
        <circle cx="121" cy="143" r="13" fill="#B86F3C" />
      </g>
      <g className="courier-body">
        <ellipse cx="150" cy="168" rx="55" ry="62" fill="#C9844A" />
        <ellipse cx="150" cy="177" rx="34" ry="43" fill="#F2D4AC" />
        <circle cx="150" cy="88" r="54" fill="#C9844A" />
        <path d="M98 60 117 14 136 60M166 60 185 14 204 60" fill="#B86F3C" />
        <rect className="courier-ear" x="146" y="10" width="7" height="30" rx="4" fill="#6B4A34" transform="rotate(20 149 40)" />
        <ellipse cx="130" cy="86" rx="5" ry="7" fill="#30363D" />
        <ellipse cx="171" cy="86" rx="5" ry="7" fill="#30363D" />
        <g fill="#6B4A34" opacity=".85">
          <rect x="107" y="98" width="14" height="14" rx="2" transform="rotate(45 114 105)" />
          <rect x="127" y="108" width="14" height="14" rx="2" transform="rotate(45 134 115)" />
          <rect x="174" y="98" width="14" height="14" rx="2" transform="rotate(45 181 105)" />
        </g>
        <path d="M145 102q5 6 10 0" fill="none" stroke="#6B4A34" strokeWidth="3" strokeLinecap="round" />
        <g className="courier-package">
          <rect x="166" y="143" width="70" height="48" rx="8" fill="#E7C79B" />
          <path d="m195 158 9 9-9 9-9-9Z" fill="#6B4A34" />
        </g>
        <g className="courier-feet" fill="#B86F3C"><ellipse cx="119" cy="226" rx="16" ry="11" /><ellipse cx="174" cy="226" rx="16" ry="11" /></g>
      </g>
      {action === 'notify' && <circle className="courier-ripple" cx="185" cy="100" r="40" fill="none" stroke="#B86F3C" strokeWidth="3" />}
      {(action === 'store' || action === 'persist') && <g className="courier-cloud" fill="#E9E2D9"><rect x="180" y="30" width="50" height="42" rx="12" /><circle cx="195" cy="34" r="17" /><circle cx="216" cy="39" r="13" /></g>}
      {(action === 'permission' || action === 'deny') && <path className="courier-cap" d="M100 56h100l-15-22h-65Z" fill="#6B4A34" />}
    </svg>
  )
}
