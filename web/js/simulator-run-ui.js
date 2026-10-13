import {simulatorDeviceByID} from './ios-simulators.js';

function syncSimulatorRunControl(select){
 const jobID=select?.dataset?.simulatorDeviceFor||'';
 if(!jobID)return;
 const controls=select.closest('.simulator-run-controls');
 const button=controls?.querySelector('[data-install-simulator="'+CSS.escape(jobID)+'"]');
 if(!button)return;
 const valid=Boolean(select.value&&simulatorDeviceByID(select.value));
 button.disabled=select.disabled||!valid;
 button.setAttribute('aria-disabled',String(button.disabled));
 if(!valid&&select.value){
  button.title='原 Simulator 当前不可用，请先选择其他设备';
 }else{
  button.removeAttribute('title');
 }
}

function syncSimulatorRunControls(root=document){
 root.querySelectorAll('[data-simulator-device-for]').forEach(syncSimulatorRunControl);
}

export function initSimulatorRunUI(){
 const root=document.querySelector('#build-jobs');
 if(!root)return;
 root.addEventListener('change',event=>{
  const select=event.target.closest?.('[data-simulator-device-for]');
  if(select)syncSimulatorRunControl(select);
 });
 new MutationObserver(()=>syncSimulatorRunControls(root)).observe(root,{childList:true,subtree:true});
 window.addEventListener('ios-simulators-loaded',()=>syncSimulatorRunControls(root));
 window.addEventListener('simulator-selections-loaded',()=>syncSimulatorRunControls(root));
 window.addEventListener('view-changed',()=>syncSimulatorRunControls(root));
 queueMicrotask(()=>syncSimulatorRunControls(root));
}
