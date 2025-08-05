#!/usr/bin/env python3
import sys
import PyPDF2
from pathlib import Path
from dotenv import load_dotenv
import os

load_dotenv()

def extract_text_from_pdf(pdf_path, output_path):
    """Extract text from a PDF file and save it to a text file."""
    try:
        with open(pdf_path, 'rb') as file:
            reader = PyPDF2.PdfReader(file)
            
            if reader.is_encrypted and os.getenv("PASS_CC"):
                #List of passwords from env
                possible_passwords = [
                    os.getenv("PASS_CC"),
                    os.getenv("PASS_BIRTH"),
                    os.getenv("PASS_BIRTH2"),
                    os.getenv("PASS_SURNAME"),
                ]
                
                passwords = [p for p in possible_passwords if p is not None]
                
                decrypted = False
                for password in passwords:
                    try:
                        result = reader.decrypt(password)
                        if result:
                            print(f"Decrypted with password: {password}")
                            decrypted = True
                            break
                    except Exception as e:
                        print(f"Error decrypting with password {password}: {e}")
                        continue
                
                if not decrypted:
                    print("Failed to decrypt PDF with any password")
                    return False
            else:
                print("PDF is not encrypted")
                
            text_list = []
            for page_num, page in enumerate(reader.pages):
                text = page.extract_text()
                text_list.append(f"--- Page {page_num +1} ---\n{text}\n")
                
            with open(output_path, 'w', encoding='utf-8') as output_file:
                output_file.write('\n'.join(text_list))
                
            print(f"Text extracted and saved to {output_path}")
            return True
        
    except Exception as e:
        print(f"Error extracting text from {pdf_path}: {e}")
        return False

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python extract_text.py <input_pdf> <output_txt>")
        sys.exit(1)
    
    input_pdf = sys.argv[1]
    output_txt = sys.argv[2]
    
    success = extract_text_from_pdf(input_pdf, output_txt)
    sys.exit(0 if success else 1)

    