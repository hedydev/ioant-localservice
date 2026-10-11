import {api,escapeHTML,state} from './core.js';

export const simulatorState={
 devices:[],profileDevices:{},buildDevices:{},project:null,loadedAt:0,loading:null
};

function deviceLabel(device){
 const status=device.state==='Booted'?' · 已启动':'';
 return [device.name,device.runtime].filter(Boolean).join(' · ')+status;
}

export function simulatorDeviceLabel(device){
 return device?deviceLabel(device):'';
}

export function simulatorDeviceByID(id){
 return simulatorState.devices.find(device=>device.udid===id)||null;
}

export function simulatorOptions(selected='',{includeAuto=false,autoLabel='自动选择（优先已启动设备）'}={}){
 const options=[];
 if(includeAuto)options.push('<option value=""'+(!selected?' selected':'')+'>'+escapeHTML(autoLabel)+'</option>');
 for(const device of simulatorState.devices){
  options.push('<option value="'+escapeHTML(device.udid)+'"'+(device.udid===selected?' selected':'')+'>'+escapeHTML(deviceLabel(device))+'</option>');
 }
 if(selected&&!simulatorDeviceByID(selected)){
  options.push('<option value="'+escapeHTML(selected)+'" selected>原设备当前不可用 · '+escapeHTML(selected)+'</option>');
 }
 return options.join('');
}

export async function loadIOSSimulators({force=false}={}){
 if(!state.admin){
  simulatorState.devices=[];
  simulatorState.loadedAt=0;
  return [];
 }
 const fresh=!force&&simulatorState.loadedAt&&Date.now()-simulatorState.loadedAt<10000;
 if(fresh)return simulatorState.devices;
 if(simulatorState.loading)return simulatorState.loading;
 simulatorState.loading=api('/api/local/ios-simulators',{timeoutMs:15000})
  .then(result=>{
   simulatorState.devices=Array.isArray(result?.devices)?result.devices:[];
   simulatorState.loadedAt=Date.now();
   window.dispatchEvent(new CustomEvent('ios-simulators-loaded',{detail:{devices:simulatorState.devices}}));
   return simulatorState.devices;
  })
  .finally(()=>{simulatorState.loading=null;});
 return simulatorState.loading;
}

export async function loadProjectSimulatorSelections(project=state.project){
 if(!state.admin||!project){
  simulatorState.project=null;
  simulatorState.profileDevices={};
  simulatorState.buildDevices={};
  return;
 }
 const [profiles,builds]=await Promise.all([
  api('/api/projects/'+encodeURIComponent(project)+'/simulator-profile-devices').catch(()=>({})),
  api('/api/projects/'+encodeURIComponent(project)+'/simulator-build-devices').catch(()=>({}))
 ]);
 if(project!==state.project)return;
 simulatorState.project=project;
 simulatorState.profileDevices=profiles&&typeof profiles==='object'?profiles:{};
 simulatorState.buildDevices=builds&&typeof builds==='object'?builds:{};
 window.dispatchEvent(new CustomEvent('simulator-selections-loaded',{detail:{project}}));
}

export function resetSimulatorState(){
 simulatorState.project=null;
 simulatorState.profileDevices={};
 simulatorState.buildDevices={};
}
