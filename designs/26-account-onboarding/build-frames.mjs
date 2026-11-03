// Emits JSX for OpenPencil's native render tool; this is design source, not application UI.
import { writeFile } from 'node:fs/promises';
const v = name => `{designVar("color/${name}")}`;
const q = JSON.stringify;
function text(label, { size = 15, weight = 400, color = 'ink', line = 20, width = 'fill', name = label, extra = '' } = {}) {
  return `<Text name=${q(width==='hug'?'Intrinsic / '+name:name)} font="Nunito" weight={${weight}} size={${size}} lineHeight={${line}} color=${v(color)} w=${typeof width === 'number' ? `{${width}}` : q(width)} h="hug" ${extra}>{${q(label)}}</Text>`;
}
function stack(name, children, gap = 24, extra = '') { return `<Frame name=${q(name)} flex="col" w="fill" h="hug" gap={${gap}} ${extra}>${children.join('')}</Frame>`; }
function row(name, children, gap = 16, extra = '') { return `<Frame name=${q(name)} flex="row" w="fill" h="hug" gap={${gap}} items="center" ${extra}>${children.join('')}</Frame>`; }
const svg = (path, size = 20) => `<svg viewBox="0 0 24 24" size={${size}}><path d=${q(path)} stroke="#101010" strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round" fill="none" /></svg>`;
const back = svg('M15 5 8 12l7 7M8 12h13');
const warning = svg('M12 8v5m0 3h.01M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0', 18);
const mail = svg('M4 5h16v14H4zM4 6l8 7 8-7', 32);
const laptop = svg('M5 4h14v12H5zM5 16l-3 4h20l-3-4', 40);
function button(label, kind = 'primary', name = label) {
  const bg = kind === 'primary' || kind === 'busy' ? 'blue' : kind === 'quiet' ? 'white' : 'surface';
  const fg = kind === 'primary' || kind === 'busy' ? 'white' : kind === 'disabled' ? 'muted' : 'ink';
  const spinner=kind==='busy'?'<svg name="Activity / rotate only while request is active" viewBox="0 0 24 24" size={18}><path d="M12 3a9 9 0 1 1-9 9" fill="none" stroke="#FFFFFF" strokeWidth={2} strokeLinecap="round" /></svg>':'';
  return `<Frame name=${q(`Button / ${name}`)} flex="row" w="fill" h={48} bg=${v(bg)} rounded={24} gap={8} items="center" justify="center">${spinner}${text(label, { weight:600, color:fg, width:'hug' })}</Frame>`;
}
function error(label) { return row('Inline error / live announcement', [warning, text(label, { size:13, line:18 })], 8, 'items="start"'); }
function field(label, value, { code = false, invalid = false, focus = false } = {}) {
  return stack(`Field / ${label}`, [text(label, { size:13, line:18, color:'muted' }), `<Frame name=${q(`Input / ${label} / single input with autofill`)} flex="row" w="fill" h={52} px={16} items="center" bg=${v('white')} stroke=${v(focus ? 'blue' : invalid ? 'ink' : 'border')} strokeWidth={${focus ? 2 : 1}} rounded={14}>${text(value, { size:code ? 20 : 15, weight:code ? 600 : 400, line:code ? 28 : 20, color:value ? 'ink':'muted', extra:code ? 'letterSpacing={3}' : '' })}</Frame>`], 8);
}
function header(title, subtitle) { return stack('Heading', [text(title, { size:28, weight:700, line:36 }), ...(subtitle ? [text(subtitle, { color:'muted' })] : [])], 8); }
function textAction(label,{width=80,size=13,color='muted'}={}) { return `<Frame name=${q(`Link / ${label} / 48px target`)} w={${width}} h={48} flex="row" items="center" justify="center">${text(label,{size,line:20,color,weight:color==='blue'?600:400,width:'hug'})}</Frame>`; }
function google() { return `<Frame name="Google action / 48px touch target" flex="row" w="fill" h={48} bg="#F2F2F2" rounded={24} items="center" justify="center"><Frame name="Google brand asset / official neutral pill" w={180} h={40} /></Frame>`; }
const divider = row('Alternative method', ['<Frame w="fill" h={1} bg='+v('line')+' />', text('or', {size:13, line:18, color:'muted', width:'hug'}), '<Frame w="fill" h={1} bg='+v('line')+' />'],16);
const returnToEmail = () => row('Recovery actions / 48px targets', [button('Resend code', 'quiet'), button('Change email', 'quiet')], 24);
const data = [];
const cases = [
  ['entry', 'Welcome', 'Sign in or create an account.'],
  ['entry-invalid', 'Welcome', 'Sign in or create an account.'],
  ['sending', 'Sending your code', 'mika@example.com'],
  ['code', 'Check your email', 'mika@example.com'],
  ['code-wrong', 'Check your email', 'mika@example.com'],
  ['code-expired', 'Code expired', 'mika@example.com'],
  ['code-limited', 'Try again shortly', 'mika@example.com'],
  ['send-failed', 'Couldn’t send the code', 'mika@example.com'],
  ['link-confirm', 'Continue to Pinelink', 'mika@example.com'],
  ['link-other-device', 'Continue here?', 'mika@example.com'],
  ['link-expired', 'This link expired', 'Request another code to continue.'],
  ['link-used', 'This link was used', 'Sign in again to use this device.'],
  ['google-wait', 'Continue with Google', 'Finish signing in in your browser.'],
  ['google-cancelled', 'Sign-in cancelled', 'Your files are still here.'],
  ['offline', 'You’re offline', ''],
  ['session-expired', 'Sign in again', 'Your cloud session has expired.'],
  ['account', 'Account', 'mika@example.com'],
  ['link-method', 'Connect your email', 'Verify your email to add this sign-in method.'],
  ['link-email-proof', 'Connect your sign-in methods', 'Confirm the email on your existing account.'],
  ['link-google-proof', 'Use your existing sign-in', 'Continue with Google to connect this email.'],
  ['signout', 'Sign out?', 'Cloud activity on this device will stop.'],
  ['code-checking', 'Check your email', 'mika@example.com'],
];
function content(key, title, subtitle, compact = false) {
  if(key.startsWith('entry')) return stack('Account entry', [header(title,subtitle), stack('Authentication methods', [google(),divider,stack('Email method', [field('Email',key==='entry-invalid'?'mika@':'', {invalid:key==='entry-invalid'}), ...(key==='entry-invalid'?[error('Enter a valid email address.')]:[]),button('Continue with email')],16)],24), button('Continue offline','quiet'), row('Policy links', [textAction('Terms'),textAction('Privacy')],16,'justify="center"')],24);
  if(key==='sending')return stack('Sending / server confirmation pending',[header(title,subtitle),stack('Mail request',[field('8-digit code','',{code:true}),button('Sending…','disabled'),button('Change email','quiet')],24)],32);
  if(key==='code-expired')return stack('Expired code / same email retained',[header(title,subtitle),button('Send new code'),button('Change email','quiet')],24);
  if(key==='code-checking')return stack('Code verification / server result pending',[header(title,subtitle),field('8-digit code','1234 5678',{code:true}),button('Checking…','busy')],24);
  if(['code','code-wrong','code-expired','code-limited'].includes(key)) {
    let note=key==='code-wrong'?'That code didn’t match. Try again.':key==='code-expired'?'Send a new code to continue.':key==='code-limited'?'Too many attempts.':null;
    const recovery=key==='code-expired'?button('Change email','quiet'):row('Recovery actions / server cooldown',[button(key==='code-limited'?'Resend in 02:14':'Resend in 00:42','disabled'),button('Change email','quiet')],24);
    return stack('Email code / sign-in and registration', [header(title,subtitle), stack('Verification', [field('8-digit code',key==='code-wrong'?'1234 5670':key==='code'&&compact?'1234 5678':'',{code:true,invalid:!!note,focus:compact}), ...(note?[error(note)]:[]), button(key==='code-expired'?'Send new code':'Continue',key==='code-expired'||key==='code-wrong'||compact?'primary':'disabled')],24), recovery],24);
  }
  if(key==='send-failed')return stack('Mail delivery failure',[header(title,subtitle),error('Try again. Your email is saved.'),button('Try again'),button('Change email','quiet')],24);
  if(key==='link-confirm'||key==='link-other-device')return stack('Email link / explicit confirmation',[header(title,subtitle),`<Frame w="fill" h={72} flex="row" items="center" justify="center">${laptop}</Frame>`,button(key==='link-other-device'?'Continue on this device':'Continue'),button('Use the email code','quiet')],24);
  if(key==='link-expired'||key==='link-used')return stack('Link recovery',[header(title,subtitle),button('Back to sign in')],32);
  if(key==='google-wait')return stack('Google browser handoff',[header(title,subtitle),google(),button('Cancel','quiet')],24);
  if(key==='google-cancelled')return stack('Provider cancellation',[header(title,subtitle),google(),button('Continue with email','secondary'),button('Continue offline','quiet')],24);
  if(key==='offline')return stack('Offline alternative',[header(title,subtitle),button('Continue offline'),button('Try again','quiet')],24);
  if(key==='session-expired')return stack('Expired session / local files retained',[header(title,subtitle),google(),button('Continue with email','secondary'),button('Continue offline','quiet')],24);
  if(key==='account')return stack('Account settings',[header(title,subtitle),stack('Sign-in methods',[row('Google',[text('Google',{weight:600}),text('Connected',{size:13,line:18,color:'muted',width:'hug'})],16),'<Frame w="fill" h={1} bg='+v('line')+' />',row('Email',[text('Email',{weight:600}),textAction('Add',{width:64,size:15,color:'blue'})],16)],20),button('Sign out','secondary')],32);
  if(key==='link-method')return stack('Explicit linking / fresh account proof',[header(title,subtitle),field('Email','mika@example.com'),button('Send code'),button('Cancel','quiet')],24);
  if(key==='link-email-proof')return stack('Linking to existing email identity',[header(title,subtitle),field('8-digit code','',{code:true}),button('Continue','disabled'),button('Resend code','quiet')],24);
  if(key==='link-google-proof')return stack('Linking to existing Google identity',[header(title,subtitle),google(),button('Cancel','quiet')],24);
  return stack('Sign-out confirmation',[header(title,subtitle),button('Sign out'),button('Stay signed in','quiet')],24);
}
function screen(key,title,subtitle,{mobile=false,width=390,keyboard=false}={}){
  const w=mobile?width:1200,h=mobile?(width===320?768:844):800,cw=mobile?width-40:360,cx=(w-cw)/2;
  const y=mobile?(keyboard?112:152):160;
  const hasBack=!key.startsWith('entry');
  const brandX=mobile?(hasBack?76:20):(hasBack?92:32),brandY=mobile?60:32;
  let children=`<Frame name="Brand" x={${brandX}} y={${brandY}} w={180} h={32} flex="row">${text('pinelink',{size:24,weight:700,line:32})}</Frame>`;
  if(hasBack)children+=`<Frame name="Back / accessible icon button" x={${mobile?12:24}} y={${mobile?52:24}} w={48} h={48} flex="row" items="center" justify="center">${back}</Frame>`;
  if(mobile)children+=`<Frame name="System time" x={20} y={12} w={80} h={20} flex="row">${text('9:41',{size:14,line:20,width:'hug'})}</Frame><Frame name="Home indicator" x={${(w-126)/2}} y={${h-20}} w={126} h={4} rounded={2} bg=${v('ink')} />`;
  children+=`<Frame name="Form column / semantic groups" x={${cx}} y={${y}} w={${cw}} h="hug" flex="col">${content(key,title,subtitle,keyboard)}</Frame>`;
  if(keyboard){children+=`<Frame name="System numeric keyboard / viewport reference" x={0} y={554} w={${w}} h={290} bg=${v('surface')}>${[1,2,3,4,5,6,7,8,9,0].map((n,i)=>`<Frame x={${8+(i===9?1:i%3)*128}} y={${12+Math.floor(i/3)*54}} w={118} h={46} rounded={6} bg=${v('white')} flex="row" items="center" justify="center">${text(String(n),{size:22,line:28,width:'hug'})}</Frame>`).join('')}</Frame>`;}
  return `<Frame name=${q(`${mobile?'Mobile':'Desktop'} / ${key}${width===320?' / 320px':''}${keyboard?' / keyboard':''}`)} w={${w}} h={${h}} bg=${v('white')} overflow="hidden">${children}</Frame>`;
}
for(let i=0;i<cases.length;i++)for(const mobile of [false,true]){
  const [key,title,subtitle]=cases[i];
  const page=['entry','entry-invalid','sending','code','code-wrong','code-expired','code-limited','send-failed','link-confirm','link-other-device','code-checking'].includes(key)?'core':'states';
  const ordinal=data.filter(d=>d.page===page).length;
  const pair=Math.floor(ordinal/2);
  data.push({key:`${mobile?'mobile':'desktop'}-${key}`,page,x:(pair%3)*1740+(mobile?1264:0),y:Math.floor(pair/3)*980,jsx:screen(key,title,subtitle,{mobile})});
}
data.push({key:'mobile-entry-320',page:'core',x:0,y:3920,jsx:screen('entry','Welcome','Sign in or create an account.',{mobile:true,width:320})});
data.push({key:'mobile-code-keyboard',page:'core',x:420,y:3920,jsx:screen('code','Check your email','mika@example.com',{mobile:true,keyboard:true})});
const mailJsx=`<Frame name="Email / code and short link" w={480} h={640} bg=${v('white')}><Frame x={32} y={32} w={416} h="hug" flex="col" gap={32}>${text('pinelink',{size:24,weight:700,line:32})}${header('Continue to Pinelink','Sign in or create your account.')}${button('Continue to Pinelink')}${stack('Code alternative',[text('Or enter this code',{color:'muted',size:13,line:18}),text('1234 5678',{size:28,weight:700,line:36,extra:'letterSpacing={3}'}),text('Expires in 15 minutes.',{color:'muted',size:13,line:18})],12)}${text('If you didn’t request this, you can ignore this email.',{size:13,line:18,color:'muted'})}</Frame></Frame>`;
data.push({key:'email-preview',page:'contract',x:0,y:0,jsx:mailJsx});
const flow=(title,steps)=>stack(title,[text(title,{size:20,weight:700,line:28}),...steps.map((s,i)=>row(`${i+1}. ${s}`,[`<Frame w={28} h={28} bg=${v(i===steps.length-1?'blue':'surface')} rounded={14} flex="row" items="center" justify="center">${text(String(i+1),{width:'hug',weight:600,color:i===steps.length-1?'white':'ink'})}</Frame>`,text(s)],16))],16);
data.push({key:'short-flow-contract',page:'contract',x:560,y:0,jsx:`<Frame name="Contract / activation and identity" w={780} h={980} bg=${v('white')} p={32} flex="col" gap={32}>${header('Shortest path to transfer','Design and backend contract · v26')}${row('Normal paths',[flow('Google',['Choose Google','Provider confirmation','Device graph']),flow('Email',['Enter email','Verify code or link','Device graph'])],32,'items="start"')}${flow('Recovery stays in place',['Keep the original destination','Use server resend and expiry times','Retry without losing email or local files'])}${flow('Account boundaries',['Verified provider subject identifies Google accounts','Linking methods requires fresh proof','Offline LAN does not require cloud sign-in'])}${text('No required profile, server configuration, tutorial, payment or success page.',{size:15,line:20,color:'muted'})}${text('Short link: /a/ + 22 random URL-safe characters. No email, JWT, session token or nested next parameter.',{size:15,line:20,color:'muted'})}</Frame>`});
if(process.argv[2])await writeFile(process.argv[2],JSON.stringify(data));else process.stdout.write(JSON.stringify(data));
