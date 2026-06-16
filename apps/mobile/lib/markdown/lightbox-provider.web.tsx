/**
 * Web lightbox provider.
 *
 * The native provider uses `react-native-image-viewing`, whose published
 * package only includes ios/android ImageItem entrypoints. Expo Web cannot
 * resolve that package, so Web uses this small modal implementation instead.
 */
import { createContext, use, useState, type ReactNode } from "react";
import { Image as ExpoImage } from "expo-image";
import { Modal, Pressable, StyleSheet, View } from "react-native";

interface LightboxApi {
  open: (uri: string) => void;
}

const LightboxContext = createContext<LightboxApi>({
  open: () => {
    // No-op fallback when used outside provider.
  },
});

export function useLightbox(): LightboxApi {
  return use(LightboxContext);
}

export function LightboxProvider({ children }: { children: ReactNode }) {
  const [uri, setUri] = useState<string | null>(null);
  const open = (nextUri: string) => setUri(nextUri);
  const close = () => setUri(null);

  return (
    <LightboxContext.Provider value={{ open }}>
      {children}
      <Modal transparent visible={uri !== null} animationType="fade" onRequestClose={close}>
        <Pressable style={styles.backdrop} onPress={close}>
          <View style={styles.imageShell}>
            {uri ? (
              <ExpoImage
                source={{ uri }}
                style={styles.image}
                contentFit="contain"
              />
            ) : null}
          </View>
        </Pressable>
      </Modal>
    </LightboxContext.Provider>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    alignItems: "center",
    backgroundColor: "rgba(0, 0, 0, 0.9)",
    flex: 1,
    justifyContent: "center",
    padding: 24,
  },
  imageShell: {
    height: "100%",
    maxHeight: 900,
    maxWidth: 1200,
    width: "100%",
  },
  image: {
    height: "100%",
    width: "100%",
  },
});
