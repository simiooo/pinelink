/* Design-preview motion owner. No request, permission or domain mutation APIs. */
(() => {
  const defaults = {x:0,y:0,scale:1,opacity:1};
  const spring = (start,target,velocity,t) => {
    const omega=Math.sqrt(420),decay=17,damped=Math.sqrt(420-decay*decay);
    const a=start-target,b=(velocity+decay*a)/damped,e=Math.exp(-decay*t),co=Math.cos(damped*t),si=Math.sin(damped*t);
    return {value:target+e*(a*co+b*si),velocity:e*((-a*damped*si+b*damped*co)-decay*(a*co+b*si))};
  };
  function curve(p){let low=0,high=1,t=p;for(let i=0;i<14;i++){t=(low+high)/2;const x=3*(1-t)*(1-t)*t*.2+t*t*t;if(x<p)low=t;else high=t;}return 3*(1-t)*t*t+t*t*t;}
  function css(v){return {transform:`translate(${v.x}px,${v.y}px) scale(${v.scale})`,opacity:Math.max(0,Math.min(1,v.opacity))};}
  class MotionOwner {
    constructor(){this.records=new Map();this.styleRecords=new Map();this.animations=new Set();this.loops=new Map();this.reduced=false;this.paused=false;this.visible=true;this.speed=1;this.retargetCount=0;}
    set(el,values){const old=this.records.get(el);if(old?.animation){this.animations.delete(old.animation);old.animation.cancel();}const v={...defaults,...values};Object.assign(el.style,css(v));this.records.set(el,{el,start:v,target:v,velocity:{},duration:0,kind:'state',animation:null});}
    sample(rec){
      if(!rec.duration||!rec.animation)return {value:{...rec.target},velocity:{}};
      const ms=Math.max(0,Math.min(rec.duration,(Number(rec.animation.currentTime)||0)-(rec.delay||0))),p=ms/rec.duration,value={},velocity={};
      for(const key of Object.keys(defaults)){
        if(rec.kind==='spring'&&key!=='opacity'){const s=spring(rec.start[key],rec.target[key],rec.velocity[key]??0,ms/1000);value[key]=s.value;velocity[key]=s.velocity;}
        else{const e=curve(p);value[key]=rec.start[key]+(rec.target[key]-rec.start[key])*e;velocity[key]=0;}
      }
      if(p===1)return {value:{...rec.target},velocity:{}};return {value,velocity};
    }
    to(el,values,{duration=160,kind='state',delay=0,velocity}={}){
      const old=this.records.get(el),sample=old?this.sample(old):{value:{...defaults},velocity:{}},target={...sample.value,...values};
      if(old?.animation){this.retargetCount++;this.animations.delete(old.animation);old.animation.cancel();}
      if(this.reduced||duration===0){this.set(el,target);return Promise.resolve();}
      const rec={el,start:sample.value,target,velocity:velocity??sample.velocity,duration,delay,kind,animation:null};
      const frames=[];for(let i=0;i<=36;i++){const p=i/36,v={};for(const key of Object.keys(defaults)){v[key]=kind==='spring'&&key!=='opacity'?spring(rec.start[key],target[key],rec.velocity[key]??0,p*duration/1000).value:rec.start[key]+(target[key]-rec.start[key])*curve(p);}if(i===36)Object.assign(v,target);frames.push({...css(v),offset:p});}
      const a=el.animate(frames,{duration,delay,fill:'both',easing:'linear'});rec.animation=a;this.records.set(el,rec);this.animations.add(a);a.playbackRate=this.speed;if(this.paused)a.pause();
      return a.finished.then(()=>{if(this.records.get(el)===rec){Object.assign(el.style,css(target));rec.animation=null;}this.animations.delete(a);a.cancel();}).catch(()=>{this.animations.delete(a);});
    }
    loop(el,keyframes,{duration=1800,staticStyle={}}={}){
      const previous=this.loops.get(el);if(previous){previous.wanted=true;this.syncLoops();return;}
      const activeStyle=Object.fromEntries(Object.keys(staticStyle).map(key=>[key,el.style[key]]));
      this.loops.set(el,{el,keyframes,duration,staticStyle,activeStyle,wanted:true,animation:null});this.syncLoops();
    }
    style(el,property,value,duration=120){
      let properties=this.styleRecords.get(el);if(!properties){properties=new Map();this.styleRecords.set(el,properties);}
      const previous=properties.get(property),from=getComputedStyle(el)[property];
      if(previous?.animation){this.animations.delete(previous.animation);previous.animation.cancel();}
      el.style[property]=value;const rec={value,animation:null};properties.set(property,rec);
      if(this.reduced||duration===0||from===value)return;
      const a=el.animate([{[property]:from},{[property]:value}],{duration,easing:'cubic-bezier(.2,0,0,1)',fill:'both'});rec.animation=a;this.animations.add(a);a.playbackRate=this.speed;if(this.paused)a.pause();
      a.finished.then(()=>{if(properties.get(property)===rec){el.style[property]=value;rec.animation=null;}this.animations.delete(a);a.cancel();}).catch(()=>this.animations.delete(a));
    }
    stopLoop(el){const r=this.loops.get(el);if(!r)return;r.wanted=false;r.animation?.cancel();Object.assign(el.style,r.staticStyle);this.loops.delete(el);}
    syncLoops(){for(const r of this.loops.values()){
      if(this.reduced){r.animation?.cancel();r.animation=null;Object.assign(r.el.style,r.staticStyle);continue;}
      Object.assign(r.el.style,r.activeStyle);
      if(!r.animation){r.animation=r.el.animate(r.keyframes,{duration:r.duration,iterations:Infinity,easing:'linear'});r.animation.playbackRate=this.speed;}
      if(this.paused||!this.visible||!r.wanted)r.animation.pause();else r.animation.play();
    }}
    setReduced(value){this.reduced=Boolean(value);if(this.reduced){for(const rec of this.records.values()){rec.animation?.cancel();rec.animation=null;Object.assign(rec.el.style,css(rec.target));}for(const [el,properties]of this.styleRecords)for(const [property,rec]of properties){rec.animation?.cancel();rec.animation=null;el.style[property]=rec.value;}this.animations.clear();}this.syncLoops();}
    setPaused(value){this.paused=Boolean(value);for(const a of this.animations){if(value)a.pause();else a.play();}this.syncLoops();}
    setVisible(value){this.visible=Boolean(value);this.syncLoops();}
    setSpeed(value){this.speed=value;for(const a of this.animations)a.playbackRate=value;for(const l of this.loops.values())if(l.animation)l.animation.playbackRate=value;}
    clear(){for(const a of this.animations)a.cancel();for(const l of this.loops.values())l.animation?.cancel();this.animations.clear();this.loops.clear();this.records.clear();this.styleRecords.clear();}
    forget(root){for(const [el,rec] of this.records){if(el===root||root.contains(el)){if(rec.animation){rec.animation.cancel();this.animations.delete(rec.animation);}this.records.delete(el);}}for(const [el,props]of this.styleRecords)if(el===root||root.contains(el)){for(const rec of props.values()){rec.animation?.cancel();this.animations.delete(rec.animation);}this.styleRecords.delete(el);}for(const [el,l] of this.loops){if(el===root||root.contains(el)){l.animation?.cancel();this.loops.delete(el);}}}
    diagnostics(){return {activeAnimations:this.animations.size,loopDefinitions:this.loops.size,runningLoops:[...this.loops.values()].filter(l=>l.animation?.playState==='running').length,reduced:this.reduced,paused:this.paused,visible:this.visible,retargets:this.retargetCount};}
  }
  window.PinelinkMotionOwner=MotionOwner;
})();
