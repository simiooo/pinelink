import {readFile,writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
const base=new URL('./',import.meta.url),read=async name=>JSON.parse(await readFile(new URL(name,base),'utf8'));
const groups=[
 ['Controls',[
 ['primary-action','Primary action','press'],['secondary-action','Secondary action','press'],['icon-action','Icon action','press'],['nav-link','Navigation link','navigation'],['tooltip','Tooltip','tooltip'],['menu','More menu','menu'],['tab','Tab underline','tabs'],['input-focus','Field focus','field'],['input-validation','Field validation','validation'],['secret-toggle','Show or hide password','icon'],['checkbox','Remember checkbox','check'],['language-option','Language option check','check'],['select','System select','native'],['chip','Action chip','press'],['toast','Result feedback','feedback'],['loading','Loading state','busy'],['error','Recoverable error','validation'],['empty-state','Empty state','surface']]],
 ['Devices',[
 ['device-arrival','Device arrives','node'],['device-departure','Device leaves','node'],['device-hover','Device hover or focus','node'],['device-press','Device press','press'],['device-select','Select a device','selection'],['device-unselect','Return to graph','selection'],['device-online','Device comes online','badge'],['device-waiting','Device waiting','badge'],['device-offline','Device offline','badge'],['device-key-change','Device key changed','validation'],['cloud-node','Cloud source node','cloud'],['connection-line','Connection relationship','graph'],['node-actions','Orbit actions','orbit'],['discovery','Local discovery','busy'],['reconnect','Reconnect a known device','queue'],['trust-confirm','Confirm device trust','receipt']]],
 ['Drag and drop',[
 ['drag-near','Approach a drop target','drag'],['drag-over','Enter a valid target','drag'],['drag-leave','Leave a target','drag'],['drag-switch','Switch between targets','drag'],['drop-accept','Drop to review','drop'],['drop-reject','Unsupported drop','validation'],['drop-empty','Drop with no usable file','validation'],['drag-cancel','Cancel a drag','drag']]],
 ['Transfers',[
 ['prepare-validating','Local file validation','busy'],['send-pending','Send request pending','busy'],['start-direct','Direct transfer begins','graph'],['start-relay','Relay transfer begins','graph'],['progress-ack','Confirmed-byte boundary','progress'],['flow-active','Transfer flow','flow'],['pause-pending','Pause request pending','busy'],['paused','Transfer paused','flow'],['resume-pending','Resume request pending','busy'],['resumed','Transfer resumes','flow'],['verify','Verify and commit','busy'],['delivered','Delivery receipt confirmed','receipt'],['route-switch','Direct and relay route switch','graph'],['cloud-stored','Encrypted cloud staging ready','cloud'],['retry','Retry a failed operation','queue'],['declined','Recipient declines','feedback'],['cancel-pending','Cancel request pending','busy'],['cancelled','Cancellation confirmed','surface']]],
 ['Surfaces',[
 ['desktop-panel','Desktop floating panel','surface'],['mobile-sheet','Mobile bottom sheet','sheet'],['fullpage','Full secondary page','navigation'],['backdrop','Inactive graph context','selection'],['keyboard','System keyboard','native'],['dialog','Confirmation dialog','surface'],['context-menu','Context menu','menu'],['detail-expand','Expand transfer details','surface']]],
 ['Files and text',[
 ['grid-load','History grid appears','list'],['filter-change','History filter changes','list'],['item-select','Select a history item','selection'],['item-open','Open local content','navigation'],['availability-change','Availability badge changes','badge'],['item-remove','Remove a confirmed entry','list'],['cloud-restore','Restore from a cloud node','cloud'],['keep-copy','Keep an authorized copy','receipt'],['text-mode','Files and Text switch','tabs'],['text-compose','Plain-text composition','static'],['text-send','Text send request','busy'],['text-receive','Text received and committed','receipt'],['text-copy','Copy text succeeds','copy'],['text-copy-fail','Copy denied or unavailable','validation']]],
 ['Account and trust',[
 ['login-switch','Choose a login method','navigation'],['google-pending','Google authorization pending','busy'],['credential-focus','Credential field focus','field'],['form-submit','Password form submit','busy'],['email-sent','Verification email sent','feedback'],['code-verify','Verify mailbox proof','receipt'],['resend-timer','Resend countdown','static'],['reset-password','Password reset result','receipt'],['account-switch','Account profile switch','navigation'],['logout','Leave an account','navigation'],['invite-create','Create a one-use invite','busy'],['invite-scan','Native camera scan','native'],['invite-compare','Compare stable codes','static'],['invite-wait','Wait for peer approval','busy'],['invite-expired','Invite expires','feedback'],['remember-trust','Remember a trusted device','check'],['forget-device','Forget a peer','list']]],
 ['Offline and settings',[
 ['offline-indicator','Network becomes unavailable','badge'],['offline-draft','Draft saved locally','feedback'],['offline-queue','Intent waits offline','queue'],['online-catchup','Reconnect and catch up','queue'],['conflict-reconcile','Reconcile a version conflict','validation'],['service-switch','Switch isolated service profile','navigation'],['language-switch','Localize existing content','static'],['pwa-install','System install prompt','native'],['pwa-update','Update becomes available','feedback'],['quota-change','Storage usage changes','progress']]],
];
const recipeSettings={
 press:['80ms down · spring release','Scale 1 → .97 → 1; icon targets keep their 48px hit area.','Immediate pressed color; no scale.'],
 icon:['120ms','Crossfade icon states; do not animate password characters or provider logos.','Immediate correct icon.'],
 tooltip:['120ms enter / 80ms exit','Opacity and 4px approach after deliberate hover/focus; Escape dismisses.','Immediate tooltip after the same intent delay.'],
 menu:['160ms enter / 120ms exit','Open from the trigger; focus and arrow keys stay usable before animation ends.','Immediate visible menu and focus.'],
 tabs:['160ms','Underline follows the selected target; content fade only, no lateral text sweep.','Underline and selected state update immediately.'],
 field:['120ms','Border/tint respond to focus; label, value and cursor remain stable.','Immediate focus border and static ring.'],
 validation:['160ms','Inline error and icon appear; reserve local space, keep focus and input.','Immediate error, same recovery action.'],
 check:['120ms','Check path draws once after the actual local/committed selection change.','Static selected check.'],
 feedback:['160ms enter / 120ms exit','Short result transition; never infer success from animation duration.','Immediate persistent result.'],
 busy:['120ms state change','Stable-width action; activity only while truly pending, cancellable immediately.','Static busy icon and label.'],
 node:['180ms enter / 120ms leave','Opacity + 8px approach; reflow only affected peers and never replay on polling.','Immediate stable geometry and state.'],
 selection:['180ms','Selected node 64→96px (mobile 56→80); others settle at .13 opacity.','Immediate selected geometry and outline.'],
 badge:['120ms','Small overlaid icon crossfade; no repeated pulse or status paragraph.','Immediate icon with tooltip/accessible name.'],
 cloud:['220ms','Cloud symbol identifies the actual source/target leg; Stored is distinct from Delivered.','Static correct cloud leg and status.'],
 graph:['180ms','Keep two nodes and one curve; route changes do not reset acknowledged bytes.','Immediate graph with current ledger.'],
 orbit:['180ms','At most 24ms offsets, 180ms total; actions never wait for the sequence.','Actions appear together.'],
 drag:['120ms enter / 100ms leave','One eligible halo, slight target scale; handoff retargets without flicker.','Static halo plus icon; same drop area.'],
 drop:['140ms','End system drag image, compress target briefly, then open review; no fake upload.','Open review directly.'],
 progress:['120ms bounded correction','Blue tracks confirmed bytes only; numeric values change immediately, never count up.','Immediate boundary.'],
 flow:['1800ms loop','One sheen clipped to real blue progress; pause/hidden/offscreen stops it.','No sheen; persistent running/paused semantics.'],
 receipt:['220ms','One check/outline settle only after required proof/commit/receipt; dedupe by event ID.','Static confirmed result.'],
 surface:['160ms modal / 120ms exit','Fade and gentle scale from its origin; removal/focus never depend on transitionend.','Immediate surface and focus.'],
 sheet:['220ms enter / 160ms exit','24px approach; direct manipulation follows the pointer, release keeps velocity.','Immediate geometry; native gesture remains usable.'],
 navigation:['280ms / 16px','Parent-child relationship; Back reverses direction, preserved content does not remount.','Immediate route with restored focus.'],
 list:['180ms','Keyed items preserve location/selection; cap initial stagger at 3 items / 48ms.','Immediate keyed result, no stagger.'],
 copy:['160ms','Copy → check only after the clipboard promise resolves; failure keeps selectable text.','Immediate actual result.'],
 queue:['160ms','Offline intent stays pending; catch-up reconciles grant/epoch before success feedback.','Static pending/committed icon and text.'],
 native:['Platform-owned','Keep native keyboard, file picker, camera permission, select and install prompt motion.','Let the platform apply its accessibility settings.'],
 static:['No content motion','Keep text, codes, caret, countdown and language reflow readable; animate only a surrounding control when relevant.','Same stable content.'],
};
const authority={
 'drop-accept':'Local content eligibility and explicit target; review is not delivery.',
 'progress-ack':'Receiver-confirmed ledger; quota uses its own authoritative usage snapshot.',
 'delivered':'Authenticated receive receipt after validation and durable commit.',
 'cloud-stored':'Required ciphertext staging commit; never a receive receipt.',
 'text-copy':'Clipboard write promise fulfilled after a user action.',
 'text-receive':'Verified, committed local text and authenticated receive receipt.',
 'keep-copy':'Independent authorized Keep reference committed.',
 'code-verify':'Authoritative challenge verification, not the check animation.',
 'email-sent':'Mail adapter has accepted the current generation; pending/failed stay distinct.',
 'trust-confirm':'Both required device proofs and explicit consent committed.',
 'paused':'Pause revision committed; a late confirmed-byte update is static and does not restart sheen.',
 'cancelled':'Authoritative cancellation and fences applied.',
 'invite-expired':'Authority expiry; code/image is disabled immediately.',
 'account-switch':'Old profile epoch aborted before new isolated profile is revealed.',
 'logout':'Credentials/old request authority revoked before decorative exit.',
 'forget-device':'Trust/grant revoked and active I/O fenced before the local row settles.',
 'service-switch':'No credentials, drafts or active transfers cross the service namespace.',
 'online-catchup':'Durable validated delta/intent commit; an online event alone is not success.',
 'item-remove':'Appropriate server/local deletion committed; selection follows a surviving key.',
};
let index=0;
const targets=groups.flatMap(([group,rows])=>rows.map(([id,name,recipe])=>({code:'M'+String(++index).padStart(3,'0'),id,name,group,recipe,mode:recipe==='native'?'native':recipe==='static'?'static':'animated',timing:recipeSettings[recipe][0],behavior:recipeSettings[recipe][1],reduced:recipeSettings[recipe][2],interrupt:'Retarget from current visible state; stale results do not overwrite a newer epoch. Explicit state changes remain usable during motion.',authority:authority[id]??'Explicit local interaction or the current authoritative operation result; never animation completion.',motion_implementation:'design prototype only; production wiring pending design adoption and feature dependencies'})));
const catalog={schema:1,version:29,issue:'SIM-898',status:'proposal',source_baseline:'545ee8425aa6ad1af6258b722efeea1212e460d0',interpretation:'Targets are distinct interaction/state audit items, not 109 reusable component definitions. Native/static items are intentional dispositions, not missing animation.',recipes:Object.fromEntries(Object.entries(recipeSettings).map(([id,[timing,behavior,reduced]])=>[id,{timing,behavior,reduced}])),targets};
await writeFile(new URL('catalog.json',base),JSON.stringify(catalog,null,2)+'\n');
const w=await read('web-inventory.json'),d=await read('flutter-inventory.json');
const classifyWeb=n=>{
 if(/Courier|SendPreparation/.test(n.file))return {disposition:'retire',targets:[],reason:'Legacy mascot or CLI preparation UI; replace through the accepted node/real-client feature unit rather than embellishing it.'};
 if(n.file.endsWith('FileIcon.tsx'))return {disposition:'static-base',targets:['availability-change','item-select'],reason:'Keep the file glyph static; selection/availability belong to its enclosing item.'};
 if(n.tag==='input'&&n.type==='"file"')return {disposition:'native',targets:['select'],reason:'System file picker; do not draw a second modal animation.'};
 if(['input','textarea'].includes(n.tag))return {disposition:'animated',targets:['input-focus','input-validation'],reason:'Stable content with focus/error feedback.'};
 if(n.tag==='select')return {disposition:'native',targets:['select','filter-change'],reason:'Native picker, immediate selection; a redesigned keyed result may transition.'};
 if(n.tag==='Link'||n.tag==='a')return {disposition:'animated',targets:['nav-link'],reason:'Subtle activation and route relationship.'};
 if(n.tag==='summary')return {disposition:'animated',targets:['menu'],reason:'Trigger and panel share one interruptible transition.'};
 if(n.events.some(x=>x.startsWith('onDrag')||x==='onDrop'))return {disposition:'replace',targets:['drag-over','drag-leave','drop-accept','drag-cancel'],reason:'Legacy large drop area maps to the explicit eligible device target; nested dragleave must not flicker.'};
 if(n.role?.includes('progressbar'))return {disposition:'replace',targets:['progress-ack'],reason:'Main transfer progress moves to the real confirmed curve; detailed/quota meters remain bounded.'};
 if(n.role?.includes('alert'))return {disposition:'animated',targets:['error'],reason:'Persistent error and accessible announcement, no page shake.'};
 if(n.role?.includes('status'))return {disposition:'animated',targets:['toast','loading'],reason:'Actual result/busy state; do not re-announce unchanged polling.'};
 return {disposition:'animated',targets:['primary-action','send-pending','error'],reason:'Shared action feedback; actual operation authority remains in its existing event handler.'};
};
const web=w.interactions.map((n,i)=>({id:'W'+String(i+1).padStart(3,'0'),...n,...classifyWeb(n)}));
const flutter=d.handlers.map((n,i)=>({id:'F'+String(i+1).padStart(3,'0'),...n,disposition:n.file.includes('send_preparation')?'retire':n.event==='onChanged'||n.event==='onSelected'?'native-or-shared-control':'animated',targets:n.file.includes('send_preparation')?[]:n.event==='onChanged'||n.event==='onSelected'?['select','input-focus','filter-change']:['primary-action','nav-link','error'],reason:n.file.includes('send_preparation')?'Legacy CLI preparation; replace with real client sending.':'Preserve the current handler authority; apply the shared control/native picker recipe.'}));
const componentDisposition=n=>/Courier/.test(n.name)?'retire-mascot':/SendPreparation/.test(n.name)?'replace-cli-preparation':/Provider|I18n|Worker|^App$|^PinelinkApp$/.test(n.name)?'nonvisual-lifecycle':/Metadata|Heading|StorageNote/.test(n.name)?'static-content':/FileIcon/.test(n.name)?'static-icon-with-stateful-host':'shared-motion-and-designed-layout';
const targetIds=new Set(targets.map(t=>t.id));
const unmapped=[...web,...flutter].filter(n=>!n.disposition||(n.disposition!=='retire'&&n.targets.length===0)||n.targets.some(id=>!targetIds.has(id)));
if(unmapped.length)throw Error('Unmapped source interactions: '+unmapped.map(n=>n.id).join(', '));
const summary={react_components:w.components.length+w.nonVisualComponents.length,react_tsx_files:w.files.length,react_jsx_sites:Object.values(w.tags).reduce((a,b)=>a+b,0),react_interaction_sites:web.length,flutter_source_files:d.files.length,flutter_widget_declarations:d.classes.filter(c=>c.base.includes('Widget')).length,flutter_state_classes:d.classes.filter(c=>c.base.startsWith('State<')).length,flutter_painters:d.classes.filter(c=>c.base.includes('Painter')).length,flutter_handler_sites:flutter.length,targets:targets.length,animated_targets:targets.filter(t=>t.mode==='animated').length,native_targets:targets.filter(t=>t.mode==='native').length,static_targets:targets.filter(t=>t.mode==='static').length,recipes:Object.keys(recipeSettings).length,unmapped_interaction_sites:unmapped.length,production_motion_delivery:false};
await writeFile(new URL('coverage.json',base),JSON.stringify({summary,web_components:[...w.components,...w.nonVisualComponents].map(n=>({...n,disposition:componentDisposition(n)})),flutter_components:d.classes.filter(c=>c.ui).map(n=>({...n,disposition:componentDisposition(n)})),web,flutter,limitations:['Flutter constructor-like tokens are a lexical inventory and include declarations/style/data objects; only Widget subclasses are counted as Widget declarations.','A controller or AnimatedBuilder name alone does not prove visual motion; the i18n AnimatedBuilder only listens for locale rebuilds.','All 70 source interaction/semantic-control sites have a disposition. Design-native containers are inventoried separately and are not falsely called independent components.']},null,2)+'\n');
const table=targets.map(t=>`| ${t.code} | ${t.name} | ${t.recipe} | ${t.timing} | ${t.mode} |`).join('\n');
await writeFile(new URL('coverage.md',base),`# 全组件与状态覆盖表\n\n\`\`\`json\n${JSON.stringify(summary,null,2)}\n\`\`\`\n\n编号表示交互／状态检查项，并非独立组件数量。每项的触发结果、权限、取消、中断和减少运动处理见 catalog.json；源码位置和处理结论见 coverage.json。设计容器的完整原生定位见 design-inventory.json。\n\n| 编号 | 交互或状态 | 共享规则 | 节奏 | 处理 |\n| --- | --- | --- | --- | --- |\n${table}\n`);
console.log(JSON.stringify(summary));
