const fs = require('node:fs');
const path = require('node:path');
const { Resvg } = require('./.render/node_modules/@resvg/resvg-js');
const root = path.join(__dirname, process.argv[2] || '13-blue-graph');
for (const name of fs.readdirSync(root).filter(n=>n.endsWith('.svg'))) {
  const board = name.endsWith('-board.svg');
  const image = new Resvg(fs.readFileSync(path.join(root,name)), {
    fitTo: board ? {mode:'width',value:name.startsWith('desktop')?1800:1720} : {mode:'zoom',value:1.5},
    font: root.endsWith('13-blue-graph') ? {loadSystemFonts:true,defaultFontFamily:'Microsoft YaHei'} : {
      loadSystemFonts:false,
      fontFiles:['C:/Windows/Fonts/segoeui.ttf','C:/Windows/Fonts/seguisb.ttf','C:/Windows/Fonts/segoeuib.ttf'],
      defaultFontFamily:'Segoe UI'
    }
  }).render();
  fs.writeFileSync(path.join(root,name.replace(/\.svg$/,'.png')),image.asPng());
}
console.log('Rendered SVG screens and review boards.');
