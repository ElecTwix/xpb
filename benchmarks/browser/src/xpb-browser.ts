import { Encoder, Decoder, SlabAllocator, compileEncoder, compileDecoder, compileAccessor, FieldType } from '../../../runtime/ts/src/browser';

// Export for browser bundle (Playwright)
if (typeof window !== 'undefined') {
  (window as any).XPB = {
    Encoder,
    Decoder,
    SlabAllocator,
    compileEncoder,
    compileDecoder,
    compileAccessor,
    FieldType
  };
}