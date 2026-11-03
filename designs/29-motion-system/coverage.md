# 全组件与状态覆盖表

```json
{
  "react_components": 12,
  "react_tsx_files": 13,
  "react_jsx_sites": 265,
  "react_interaction_sites": 47,
  "flutter_source_files": 17,
  "flutter_widget_declarations": 21,
  "flutter_state_classes": 9,
  "flutter_painters": 1,
  "flutter_handler_sites": 23,
  "targets": 109,
  "animated_targets": 101,
  "native_targets": 4,
  "static_targets": 4,
  "recipes": 29,
  "unmapped_interaction_sites": 0,
  "production_motion_delivery": false
}
```

编号表示交互／状态检查项，并非独立组件数量。每项的触发结果、权限、取消、中断和减少运动处理见 catalog.json；源码位置和处理结论见 coverage.json。设计容器的完整原生定位见 design-inventory.json。

| 编号 | 交互或状态 | 共享规则 | 节奏 | 处理 |
| --- | --- | --- | --- | --- |
| M001 | Primary action | press | 80ms down · spring release | animated |
| M002 | Secondary action | press | 80ms down · spring release | animated |
| M003 | Icon action | press | 80ms down · spring release | animated |
| M004 | Navigation link | navigation | 280ms / 16px | animated |
| M005 | Tooltip | tooltip | 120ms enter / 80ms exit | animated |
| M006 | More menu | menu | 160ms enter / 120ms exit | animated |
| M007 | Tab underline | tabs | 160ms | animated |
| M008 | Field focus | field | 120ms | animated |
| M009 | Field validation | validation | 160ms | animated |
| M010 | Show or hide password | icon | 120ms | animated |
| M011 | Remember checkbox | check | 120ms | animated |
| M012 | Language option check | check | 120ms | animated |
| M013 | System select | native | Platform-owned | native |
| M014 | Action chip | press | 80ms down · spring release | animated |
| M015 | Result feedback | feedback | 160ms enter / 120ms exit | animated |
| M016 | Loading state | busy | 120ms state change | animated |
| M017 | Recoverable error | validation | 160ms | animated |
| M018 | Empty state | surface | 160ms modal / 120ms exit | animated |
| M019 | Device arrives | node | 180ms enter / 120ms leave | animated |
| M020 | Device leaves | node | 180ms enter / 120ms leave | animated |
| M021 | Device hover or focus | node | 180ms enter / 120ms leave | animated |
| M022 | Device press | press | 80ms down · spring release | animated |
| M023 | Select a device | selection | 180ms | animated |
| M024 | Return to graph | selection | 180ms | animated |
| M025 | Device comes online | badge | 120ms | animated |
| M026 | Device waiting | badge | 120ms | animated |
| M027 | Device offline | badge | 120ms | animated |
| M028 | Device key changed | validation | 160ms | animated |
| M029 | Cloud source node | cloud | 220ms | animated |
| M030 | Connection relationship | graph | 180ms | animated |
| M031 | Orbit actions | orbit | 180ms | animated |
| M032 | Local discovery | busy | 120ms state change | animated |
| M033 | Reconnect a known device | queue | 160ms | animated |
| M034 | Confirm device trust | receipt | 220ms | animated |
| M035 | Approach a drop target | drag | 120ms enter / 100ms leave | animated |
| M036 | Enter a valid target | drag | 120ms enter / 100ms leave | animated |
| M037 | Leave a target | drag | 120ms enter / 100ms leave | animated |
| M038 | Switch between targets | drag | 120ms enter / 100ms leave | animated |
| M039 | Drop to review | drop | 140ms | animated |
| M040 | Unsupported drop | validation | 160ms | animated |
| M041 | Drop with no usable file | validation | 160ms | animated |
| M042 | Cancel a drag | drag | 120ms enter / 100ms leave | animated |
| M043 | Local file validation | busy | 120ms state change | animated |
| M044 | Send request pending | busy | 120ms state change | animated |
| M045 | Direct transfer begins | graph | 180ms | animated |
| M046 | Relay transfer begins | graph | 180ms | animated |
| M047 | Confirmed-byte boundary | progress | 120ms bounded correction | animated |
| M048 | Transfer flow | flow | 1800ms loop | animated |
| M049 | Pause request pending | busy | 120ms state change | animated |
| M050 | Transfer paused | flow | 1800ms loop | animated |
| M051 | Resume request pending | busy | 120ms state change | animated |
| M052 | Transfer resumes | flow | 1800ms loop | animated |
| M053 | Verify and commit | busy | 120ms state change | animated |
| M054 | Delivery receipt confirmed | receipt | 220ms | animated |
| M055 | Direct and relay route switch | graph | 180ms | animated |
| M056 | Encrypted cloud staging ready | cloud | 220ms | animated |
| M057 | Retry a failed operation | queue | 160ms | animated |
| M058 | Recipient declines | feedback | 160ms enter / 120ms exit | animated |
| M059 | Cancel request pending | busy | 120ms state change | animated |
| M060 | Cancellation confirmed | surface | 160ms modal / 120ms exit | animated |
| M061 | Desktop floating panel | surface | 160ms modal / 120ms exit | animated |
| M062 | Mobile bottom sheet | sheet | 220ms enter / 160ms exit | animated |
| M063 | Full secondary page | navigation | 280ms / 16px | animated |
| M064 | Inactive graph context | selection | 180ms | animated |
| M065 | System keyboard | native | Platform-owned | native |
| M066 | Confirmation dialog | surface | 160ms modal / 120ms exit | animated |
| M067 | Context menu | menu | 160ms enter / 120ms exit | animated |
| M068 | Expand transfer details | surface | 160ms modal / 120ms exit | animated |
| M069 | History grid appears | list | 180ms | animated |
| M070 | History filter changes | list | 180ms | animated |
| M071 | Select a history item | selection | 180ms | animated |
| M072 | Open local content | navigation | 280ms / 16px | animated |
| M073 | Availability badge changes | badge | 120ms | animated |
| M074 | Remove a confirmed entry | list | 180ms | animated |
| M075 | Restore from a cloud node | cloud | 220ms | animated |
| M076 | Keep an authorized copy | receipt | 220ms | animated |
| M077 | Files and Text switch | tabs | 160ms | animated |
| M078 | Plain-text composition | static | No content motion | static |
| M079 | Text send request | busy | 120ms state change | animated |
| M080 | Text received and committed | receipt | 220ms | animated |
| M081 | Copy text succeeds | copy | 160ms | animated |
| M082 | Copy denied or unavailable | validation | 160ms | animated |
| M083 | Choose a login method | navigation | 280ms / 16px | animated |
| M084 | Google authorization pending | busy | 120ms state change | animated |
| M085 | Credential field focus | field | 120ms | animated |
| M086 | Password form submit | busy | 120ms state change | animated |
| M087 | Verification email sent | feedback | 160ms enter / 120ms exit | animated |
| M088 | Verify mailbox proof | receipt | 220ms | animated |
| M089 | Resend countdown | static | No content motion | static |
| M090 | Password reset result | receipt | 220ms | animated |
| M091 | Account profile switch | navigation | 280ms / 16px | animated |
| M092 | Leave an account | navigation | 280ms / 16px | animated |
| M093 | Create a one-use invite | busy | 120ms state change | animated |
| M094 | Native camera scan | native | Platform-owned | native |
| M095 | Compare stable codes | static | No content motion | static |
| M096 | Wait for peer approval | busy | 120ms state change | animated |
| M097 | Invite expires | feedback | 160ms enter / 120ms exit | animated |
| M098 | Remember a trusted device | check | 120ms | animated |
| M099 | Forget a peer | list | 180ms | animated |
| M100 | Network becomes unavailable | badge | 120ms | animated |
| M101 | Draft saved locally | feedback | 160ms enter / 120ms exit | animated |
| M102 | Intent waits offline | queue | 160ms | animated |
| M103 | Reconnect and catch up | queue | 160ms | animated |
| M104 | Reconcile a version conflict | validation | 160ms | animated |
| M105 | Switch isolated service profile | navigation | 280ms / 16px | animated |
| M106 | Localize existing content | static | No content motion | static |
| M107 | System install prompt | native | Platform-owned | native |
| M108 | Update becomes available | feedback | 160ms enter / 120ms exit | animated |
| M109 | Storage usage changes | progress | 120ms bounded correction | animated |
