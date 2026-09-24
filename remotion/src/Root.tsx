import React from 'react';
import { Composition, Series } from 'remotion';
import { Scene01Problem } from './scenes/Scene01Problem';
import { Scene02NormalExecution } from './scenes/Scene02NormalExecution';
import { Scene03WorkerFailure } from './scenes/Scene03WorkerFailure';
import { Scene04LeaseExpiration } from './scenes/Scene04LeaseExpiration';
import { Scene05Recovery } from './scenes/Scene05Recovery';
import { Scene06Success } from './scenes/Scene06Success';
import { Scene07Evidence } from './scenes/Scene07Evidence';

const FPS = 60;
const W = 1920;
const H = 1080;

// Scene durations in seconds → frames
const s = (secs: number) => secs * FPS;

export const RemotionRoot: React.FC = () => {
  return (
    <>
      {/* Full video composition */}
      <Composition
        id="ForgeDemo"
        component={ForgeDemoVideo}
        durationInFrames={s(90)}
        fps={FPS}
        width={W}
        height={H}
      />

      {/* Individual scene previews for development */}
      <Composition id="Scene01" component={Scene01Problem}         durationInFrames={s(5)}  fps={FPS} width={W} height={H} />
      <Composition id="Scene02" component={Scene02NormalExecution} durationInFrames={s(8)}  fps={FPS} width={W} height={H} />
      <Composition id="Scene03" component={Scene03WorkerFailure}   durationInFrames={s(12)} fps={FPS} width={W} height={H} />
      <Composition id="Scene04" component={Scene04LeaseExpiration} durationInFrames={s(12)} fps={FPS} width={W} height={H} />
      <Composition id="Scene05" component={Scene05Recovery}        durationInFrames={s(18)} fps={FPS} width={W} height={H} />
      <Composition id="Scene06" component={Scene06Success}         durationInFrames={s(15)} fps={FPS} width={W} height={H} />
      <Composition id="Scene07" component={Scene07Evidence}        durationInFrames={s(20)} fps={FPS} width={W} height={H} />
    </>
  );
};

const ForgeDemoVideo: React.FC = () => (
  <Series>
    <Series.Sequence durationInFrames={s(5)}>
      <Scene01Problem />
    </Series.Sequence>
    <Series.Sequence durationInFrames={s(8)}>
      <Scene02NormalExecution />
    </Series.Sequence>
    <Series.Sequence durationInFrames={s(12)}>
      <Scene03WorkerFailure />
    </Series.Sequence>
    <Series.Sequence durationInFrames={s(12)}>
      <Scene04LeaseExpiration />
    </Series.Sequence>
    <Series.Sequence durationInFrames={s(18)}>
      <Scene05Recovery />
    </Series.Sequence>
    <Series.Sequence durationInFrames={s(15)}>
      <Scene06Success />
    </Series.Sequence>
    <Series.Sequence durationInFrames={s(20)}>
      <Scene07Evidence />
    </Series.Sequence>
  </Series>
);
