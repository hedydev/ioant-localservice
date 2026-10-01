
import {$,state} from './core.js';

function renderAutomation(){
 const project=state.projects.find(item=>item.id===state.project);
 $('#api-example').textContent=project
  ?'LOCALSERVICE_URL=http://这台Mac的局域网IP:8787 \\\nLOCALSERVICE_TOKEN_FILE=/path/to/admin-token \\\n./scripts/push.sh '+project.id+' 1.0.0 1 ios /path/to/app.ipa dev arm64'
  :'选择项目后显示命令';
}

export function initAutomation(){
 window.addEventListener('data-refreshed',renderAutomation);
 window.addEventListener('project-changed',renderAutomation);
 renderAutomation();
}
