export const vertexShader = `#version 300 es
const vec2 positions[3] = vec2[3](vec2(-1.,-1.),vec2(3.,-1.),vec2(-1.,3.));
void main(){ gl_Position=vec4(positions[gl_VertexID],0.,1.); }`;

export const reductionShader = `#version 300 es
precision highp float;
uniform sampler2D uValues;
uniform sampler2D uMetadata;
uniform int uSourceWidth;
uniform int uSourceCount;
uniform int uOldest;
uniform bool uRing;
layout(location=0) out vec4 values;
layout(location=1) out vec4 metadata;
int physical(int logical){ return uRing ? (uOldest+logical)&(uSourceWidth-1) : logical; }
void main(){
 int left=int(gl_FragCoord.x)*2;
 if(left>=uSourceCount){ values=vec4(0.); metadata=vec4(0.); return; }
 vec4 lv=texelFetch(uValues,ivec2(physical(left),0),0);
 vec4 lm=texelFetch(uMetadata,ivec2(physical(left),0),0);
 int right=left+1;
 if(right>=uSourceCount){ values=lv; metadata=lm; return; }
 vec4 rv=texelFetch(uValues,ivec2(physical(right),0),0);
 vec4 rm=texelFetch(uMetadata,ivec2(physical(right),0),0);
 values=vec4(min(lv.x,rv.x),max(lv.y,rv.y),lv.z,rv.w);
 metadata=vec4(lm.x,rm.y,rm.z,rm.w);
}`;

export const chartVertexShader = `#version 300 es
precision highp float;
uniform sampler2D uValues;
uniform sampler2D uMetadata;
uniform sampler2D uScaleValues;
uniform sampler2D uScaleMetadata;
uniform int uWidth;
uniform int uCount;
uniform int uOldest;
uniform bool uRing;
uniform int uScaleWidth;
uniform int uScaleOldest;
uniform bool uScaleRing;
uniform int uMode;
uniform int uCurveSegments;
uniform float uPixelRatio;
int physical(int logical){ return uRing ? (uOldest+logical)&(uWidth-1) : logical; }
vec4 valueAt(int logical){ return texelFetch(uValues,ivec2(physical(logical),0),0); }
vec4 metaAt(int logical){ return texelFetch(uMetadata,ivec2(physical(logical),0),0); }
vec4 scaleValue(){ int at=uScaleRing?uScaleOldest&(uScaleWidth-1):0; return texelFetch(uScaleValues,ivec2(at,0),0); }
vec4 scaleMeta(){ int at=uScaleRing?uScaleOldest&(uScaleWidth-1):0; return texelFetch(uScaleMetadata,ivec2(at,0),0); }
vec2 point(float time,float value){
 vec4 scale=scaleValue(); vec4 span=scaleMeta();
 float low=0.; float high=max(0.001, scale.y * 1.15);
 float x=span.y>span.x?(time-span.x)/(span.y-span.x):.5;
 float y=high>low?(value-low)/(high-low):.5;
 return vec2(x*2.-1.,clamp(y,0.,1.)*2.-1.);
}
void main(){
 int bucket=gl_VertexID/2; int endpoint=gl_VertexID&1;
 vec4 values=valueAt(bucket); vec4 metadata=metaAt(bucket);
 if(uMode==0){ float value=endpoint==0?values.x:values.y; gl_Position=vec4(point((metadata.x+metadata.y)*.5,value),0.,1.); return; }
 if(uMode==1){ int next=min(bucket+1,uCount-1); vec4 nm=metaAt(next); vec4 nv=valueAt(next); gl_Position=vec4(endpoint==0?point(metadata.y,values.w):point(nm.x,nv.z),0.,1.); return; }
 if(uMode==2){
   int width=uCurveSegments+2; int interval=gl_VertexID/width; int local=gl_VertexID%width;
   vec4 curVal=valueAt(interval); vec4 curMeta=metaAt(interval);
   vec4 nextVal=valueAt(interval+1); vec4 nextMeta=metaAt(interval+1);
   if(local==width-1){ gl_Position=vec4(point(nextMeta.x,nextVal.z),0.,1.); return; }
   float fraction=float(local)/float(uCurveSegments);
   float time=mix(curMeta.y,nextMeta.x,fraction);
   float value=mix(curVal.w,nextVal.z,fraction);
   if(!isnan(curMeta.z)&&!isnan(curMeta.w)&&curMeta.w>0.){
     value=curMeta.z+(curVal.w-curMeta.z)*exp(-curMeta.w*(time-curMeta.y));
   }
   gl_Position=vec4(point(time,value),0.,1.); return;
 }
 if(uMode==3){
   int eventIdx=gl_VertexID/2; int isTop=gl_VertexID&1;
   vec4 rugMeta=metaAt(eventIdx);
   float xPos=point(rugMeta.y,0.).x;
   float yPos=isTop==1?-0.88:-1.0;
   gl_Position=vec4(xPos,yPos,0.,1.); return;
 }
 if(uMode==4){
   vec4 latestMeta=metaAt(uCount-1); float x=endpoint==0?-1.:1.;
   float muVal=isnan(latestMeta.z)?0.:latestMeta.z;
   gl_Position=vec4(x,point(latestMeta.y,muVal).y,0.,1.); return;
 }
 if(uMode==5){
   int stepCount=uCurveSegments+2; int quadVerts=stepCount*2;
   int interval=gl_VertexID/quadVerts; int localPair=gl_VertexID%quadVerts;
   int stepIdx=localPair/2; int isBottom=localPair&1;
   vec4 curVal=valueAt(interval); vec4 curMeta=metaAt(interval);
   vec4 nextVal=valueAt(interval+1); vec4 nextMeta=metaAt(interval+1);
   float time=mix(curMeta.y,nextMeta.x,min(1.,float(stepIdx)/float(uCurveSegments)));
   float value=mix(curVal.w,nextVal.z,min(1.,float(stepIdx)/float(uCurveSegments)));
   if(!isnan(curMeta.z)&&!isnan(curMeta.w)&&curMeta.w>0.){
     value=curMeta.z+(curVal.w-curMeta.z)*exp(-curMeta.w*(time-curMeta.y));
   }
   if(stepIdx==stepCount-1){ time=nextMeta.x; value=nextVal.z; }
   float yVal=isBottom==1?0.:value;
   gl_Position=vec4(point(time,yVal),0.,1.); return;
 }
}
`;

export const chartFragmentShader = `#version 300 es
precision highp float;
uniform vec4 uColor;
out vec4 color;
void main(){ color=uColor; }`;
