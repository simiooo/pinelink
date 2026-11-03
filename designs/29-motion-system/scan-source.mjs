// Inventory only. Run with the repository's locked TypeScript package available.
import ts from '../../web/node_modules/typescript/lib/typescript.js';
import {readFile,readdir,writeFile} from 'node:fs/promises';
import {relative,resolve,join} from 'node:path';
const root=resolve(process.argv[2]??'.'),out=process.argv[3];
async function walk(dir){const found=[];for(const d of await readdir(dir,{withFileTypes:true})){if(d.isDirectory())found.push(...await walk(join(dir,d.name)));else if(d.name.endsWith('.tsx')&&!/\.test\./.test(d.name))found.push(join(dir,d.name));}return found.sort();}
const components=[],nonVisualComponents=[],interactions=[],files=[],tags={},motionSites=[];
for(const file of await walk(join(root,'web/src'))){
 const src=await readFile(file,'utf8'),sf=ts.createSourceFile(file,src,ts.ScriptTarget.Latest,true,ts.ScriptKind.TSX),path=relative(root,file).replaceAll('\\','/');
 const loc=n=>({file:path,line:sf.getLineAndCharacterOfPosition(n.getStart(sf)).line+1});
 const hasJSX=n=>{let yes=false;const visit=x=>{if(ts.isJsxElement(x)||ts.isJsxSelfClosingElement(x)||ts.isJsxFragment(x))yes=true;ts.forEachChild(x,visit);};visit(n);return yes;};
 function visit(n){
  if(ts.isFunctionDeclaration(n)&&n.name&&/^[A-Z]/.test(n.name.text))(hasJSX(n)?components:nonVisualComponents).push({name:n.name.text,...loc(n)});
  if(ts.isVariableDeclaration(n)&&ts.isIdentifier(n.name)&&/^[A-Z]/.test(n.name.text)&&n.initializer&&hasJSX(n.initializer))components.push({name:n.name.text,...loc(n)});
  if(ts.isJsxOpeningElement(n)||ts.isJsxSelfClosingElement(n)){
   const tag=n.tagName.getText(sf),attrs=n.attributes.properties.filter(ts.isJsxAttribute),events=attrs.map(a=>a.name.getText(sf)).filter(x=>/^on[A-Z]/.test(x)),get=name=>attrs.find(a=>a.name.getText(sf)===name)?.initializer?.getText(sf);
   tags[tag]=(tags[tag]??0)+1;
   if(events.length||['button','input','select','textarea','summary','a','Link','NavLink'].includes(tag)||get('role'))interactions.push({...loc(n),tag,events,role:get('role')??null,className:get('className')??null,type:get('type')??null});
  }
  if(ts.isCallExpression(n)&&/^(?:.*\.)?(?:animate|requestAnimationFrame)$/.test(n.expression.getText(sf)))motionSites.push({...loc(n),call:n.expression.getText(sf)});
  ts.forEachChild(n,visit);
 }
 visit(sf);files.push({file:path,lines:src.split('\n').length});
}
const css=await readFile(join(root,'web/src/styles/app.css'),'utf8');
const cssMotion=css.split('\n').flatMap((s,i)=>/animation:|transition:|@keyframes|prefers-reduced-motion/.test(s)?[{file:'web/src/styles/app.css',line:i+1,rule:s.trim()}]:[]);
const data={method:'TypeScript AST, all non-test TSX under web/src; DOM events and semantic controls; CSS motion declarations',files,components,nonVisualComponents,tags,interactions,motionSites,cssMotion};
if(out)await writeFile(out,JSON.stringify(data,null,2)+'\n');else process.stdout.write(JSON.stringify(data,null,2));
